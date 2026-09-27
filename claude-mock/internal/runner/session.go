package runner

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// defaultConfigDir is the default CLAUDE_CONFIG_DIR for mock runs.
	// Using a fixed path (not a temp dir) means session files survive between
	// invocations and the script can read history from previous turns.
	// sr:docs https://code.claude.com/docs/en/agent-sdk/sessions (CLAUDE_CONFIG_DIR)
	defaultConfigDir = "/tmp/a10n-mock"
)

// resolveConfigDir returns the effective config dir: explicit arg wins, then
// CLAUDE_CONFIG_DIR env var, then defaultConfigDir.
func resolveConfigDir(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		return v
	}
	return defaultConfigDir
}

// sessionFilePath returns the path where the session JSONL is stored, mirroring
// the real Claude Code layout:
//
//	<configDir>/projects/<encoded-resolved-cwd>/<session-id>.jsonl
//
// The encoding rule matches Claude Code exactly: every character outside
// [a-zA-Z0-9] is replaced with '-', applied to the SYMLINK-RESOLVED cwd (see
// resolveEncodingCwd) — real Claude Code resolves symlinks before encoding (verified
// empirically: a real claude run in a macOS /tmp/... dir produces transcript paths under
// /private/tmp/..., the resolved form), so a consumer that independently derives "the
// transcript path for this cwd" via its OWN symlink-resolving logic (e.g. a10n-workspace's
// session.ResolveWorkDir + StableSessionID, or a test harness's pre-seeding of a fixture
// transcript at the resolved path) must land on the SAME encoded directory the mock itself
// writes to, or the two diverge and any consumer trusting a hook payload's `transcript_path`
// field literally can never find the file. A prior version encoded cwd directly (no symlink
// resolution), which is exactly the divergence that broke transcript resolution for any
// downstream tool run from an isolated worktree.
//
// sr:docs https://code.claude.com/docs/en/agent-sdk/sessions
func sessionFilePath(configDir, cwd, sessionID string) string {
	encoded := nonAlphanumRe.ReplaceAllString(resolveEncodingCwd(cwd), "-")
	return filepath.Join(configDir, "projects", encoded, sessionID+".jsonl")
}

// resolveEncodingCwd symlink-resolves cwd for transcript-path ENCODING. Verified empirically
// against real claude: its PreToolUse payload's OWN `cwd` field is ALREADY the resolved form
// (e.g. `/private/tmp/...` on macOS, not `/tmp/...`) — real Claude Code resolves symlinks
// consistently everywhere, not just for transcript-path encoding. Falls back to the raw cwd on
// any resolution error (e.g. a not-yet-existent directory) rather than failing the whole path
// computation.
func resolveEncodingCwd(cwd string) string {
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		return resolved
	}
	return cwd
}

var nonAlphanumRe = regexp.MustCompile(`[^a-zA-Z0-9]`)

// openSessionFile creates (or appends to) the session JSONL file and returns
// the open file handle. The caller is responsible for closing it.
func openSessionFile(configDir, cwd, sessionID string) (*os.File, error) {
	path := sessionFilePath(configDir, cwd, sessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

// seedRootPromptTranscript writes the root agent's `-p` prompt as the FIRST user record
// of the session transcript — mirroring real Claude Code, whose transcript opens with the
// user prompt. A plugin's Stop hook reads the transcript's first user message (e.g. to
// harvest a10n:// links into check contexts), so the mock must persist the prompt there;
// without it the root transcript would contain only assistant/result records and that hook
// path could never be exercised. Best-effort + idempotent: only writes when the session
// file is still EMPTY (so a resume, whose transcript is already seeded, is left untouched).
//
// The record carries a uuid and an explicit NULL parentUuid — the ORIGIN shape real Claude
// Code writes and a consumer's identity walk keys on ("first uuid-carrying record whose
// parentUuid is null"). It is deterministic (`e2e-root-<sessionID>`) so a caller that
// references the root by id up front — as the sloprail harness does with its own identical
// root — lands on the same uuid; that also seeds the record chain the streamed records
// extend, so a later resume continuation chains from a real uuid rather than a fallback.
// The explicit null parent is what keeps it the origin: chainRecord fills a parent only
// when the field is ABSENT, never over a null.
func seedRootPromptTranscript(f *os.File, sessionID, cwd, prompt string) {
	if f == nil || prompt == "" {
		return
	}
	if fi, err := f.Stat(); err != nil || fi.Size() > 0 {
		return // already has content (resume / re-entry) — don't duplicate the prompt
	}
	rec := map[string]any{
		"type":       "user",
		"uuid":       "e2e-root-" + sessionID,
		"parentUuid": nil,
		"sessionId":  sessionID,
		"cwd":        cwd,
		"message":    map[string]any{"role": "user", "content": prompt},
	}
	if line, err := marshalRecord(rec); err == nil {
		appendToSession(f, line)
	}
}

// preambleTypes are the no-uuid preamble record types real Claude Code opens a
// transcript with — bookkeeping about the session rather than anything that happened
// in it. A consumer skips them (they carry no uuid and cannot join a chain) but counts
// their physical lines. Kept as a set so isPreambleType can recognise one already on
// disk (the re-entry idempotency guard).
var preambleTypes = map[string]bool{
	"custom-title":    true,
	"ai-title":        true,
	"mode":            true,
	"last-prompt":     true,
	"queue-operation": true,
	"pr-link":         true,
}

// mockPreambleRecords returns the fixed block of no-uuid preamble records the mock
// writes at the head of every FRESH transcript, so its file opens the way a real
// Claude Code session file does. Real transcripts open with these bookkeeping records
// (custom-title / mode / last-prompt / …); each carries NO uuid and NO parentUuid, so a
// reader counts the physical line but skips it as an entry. The mock previously wrote
// none — a fidelity gap (a real transcript opens with them; the mock's opened straight
// at the user prompt) — which this closes.
//
// Three representative shapes, verified byte-for-shape against a real
// ~/.claude/projects/<proj>/<session>.jsonl:
//
//	{"type":"custom-title","customTitle":"…","sessionId":"…"}
//	{"type":"mode","mode":"normal","sessionId":"…"}
//	{"type":"last-prompt","lastPrompt":"…","leafUuid":"…","sessionId":"…"}
//
// The content is deterministic — this is a mock, and nothing downstream reads the
// bookkeeping VALUES; only the "counted-but-skipped no-uuid line" shape matters. The
// sessionId is the run's own so the records look like they belong to this session.
func mockPreambleRecords(sessionID string) [][]byte {
	recs := []map[string]any{
		{"type": "custom-title", "customTitle": "a mock session", "sessionId": sessionID},
		{"type": "mode", "mode": "normal", "sessionId": sessionID},
		{"type": "last-prompt", "lastPrompt": "", "leafUuid": "", "sessionId": sessionID},
	}
	var out [][]byte
	for _, r := range recs {
		if line, err := marshalRecord(r); err == nil {
			out = append(out, line)
		}
	}
	return out
}

// seedPreamble ensures the FRESH transcript opens with the no-uuid preamble records a
// real Claude Code session file does, sitting AHEAD of the root prompt record.
//
// # Why it prepends rather than just writes
//
// The root may already be on disk when this runs. Two callers seed it: the mock's own
// seedRootPromptTranscript (which just ran, for a mock-owned fresh session) and the
// sloprail HARNESS, which pre-seeds an identical root record into the file before it
// launches the mock at all. In both cases the file's first record is the root by the
// time this runs, and the preamble has to land BEFORE it — so this reads the existing
// content and rewrites the file as [preamble…, existing…]. The order the head ends up
// in is therefore [preamble…, root, conversation…] regardless of who wrote the root.
//
// # Idempotency
//
// It is a no-op when the transcript already opens with a preamble record (a re-entry
// that re-opened a file this already processed) — so the block is written exactly once.
// An empty file gets just the preamble (a later root append then follows); but in
// practice the root is already present, since fresh runs seed it first.
//
// # Safety with the open append handle
//
// f is opened O_APPEND. Rewriting the file's content with os.WriteFile (truncate+write)
// does not invalidate the fd; a subsequent O_APPEND write seeks to the true end at write
// time — after the prepended head — so the streamed records still land in order. Best-
// effort throughout: any read/write error leaves the transcript as-is (session
// persistence must never fail the run), matching seedRootPromptTranscript.
func seedPreamble(f *os.File, sessionID string) {
	if f == nil {
		return
	}
	existing, err := os.ReadFile(f.Name())
	if err != nil {
		return
	}
	if transcriptHasPreamble(existing) {
		return // already opened with a preamble — don't write it twice
	}
	var buf []byte
	for _, line := range mockPreambleRecords(sessionID) {
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}
	buf = append(buf, existing...)
	_ = os.WriteFile(f.Name(), buf, 0o644)
}

// transcriptHasPreamble reports whether the transcript's FIRST non-empty record is a
// preamble type — the signature that seedPreamble has already run against this file.
// Only the first record is inspected: the preamble is a contiguous head block, so a
// preamble type anywhere else would not be one this wrote, and checking the head is
// both sufficient and cheap.
func transcriptHasPreamble(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec struct {
			Type string `json:"type"`
		}
		if json.Unmarshal([]byte(line), &rec) != nil {
			return false
		}
		return preambleTypes[rec.Type]
	}
	return false
}

// appendResumePromptTranscript writes a RESUME's `-p` prompt as a NEW human/user
// record — a CONTINUATION turn, not a second parentless root. This is the shape a
// real follow-up human message has: its own uuid and a NON-null parentUuid chaining
// it into the transcript already on disk, so a reader treats it as a mid-conversation
// human turn rather than a second conversation origin.
//
// Why the fresh path and the resume path differ. A fresh session opens the transcript
// with the prompt as the parentless ROOT (seedRootPromptTranscript) — the one record a
// reader scans for as the conversation's origin. A resume, by definition, already has
// that origin; its prompt is the NEXT human turn. Writing it as a second parentless
// root would give the file two origins, which an identity/normalize walk cannot make
// sense of. So this record carries parentUuid = the transcript's current last record's
// uuid where one exists (a true continuation chain), falling back to the last record
// that HAS a uuid, and finally to the root's own derived uuid — anything non-null, so it
// never reads as a root.
//
// The chain is precise when the transcript has any persisted turn: the mock now stamps
// a uuid on every FILE record (the seeded root, each streamed assistant/tool_result — see
// sessionWriter), so scanTranscriptTail's last uuid is a real record's and the
// continuation chains from it. The fallback to the root's derived id remains for the
// degenerate case of a transcript whose only content carried no uuid at all (e.g. a bare
// preamble); a fresh uuid + ANY non-null parentUuid is all the contract requires to mark
// this a human continuation rather than a second root.
//
// Re-entry duplication guard. The historical skip in seedRootPromptTranscript existed so
// the mock re-opening its own non-empty file mid-run does not double-write the root. The
// resume append must fire exactly ONCE per resume invocation, so it skips when the
// transcript's LAST user record already carries this exact prompt — the signature of a
// re-entry that already appended it — rather than blindly appending on every open.
//
// Best-effort throughout: a read or marshal failure leaves the transcript as-is (session
// persistence must never fail the run), which merely omits the continuation record — the
// same graceful degradation seedRootPromptTranscript has.
func appendResumePromptTranscript(f *os.File, sessionID, cwd, prompt string) {
	if f == nil || prompt == "" {
		return
	}
	path := f.Name()
	lastUUID, lastUserContent := scanTranscriptTail(path)
	// Re-entry guard: the last human turn is already this prompt — don't write it twice.
	if lastUserContent == prompt {
		return
	}
	parent := lastUUID
	if parent == "" {
		// No record on disk carries a uuid to chain from (the streamed records don't).
		// Any non-null parent still marks this a continuation rather than a second root;
		// derive it from the seeded root's own deterministic id so it points at a real
		// origin conceptually rather than a random token.
		parent = "e2e-root-" + sessionID
	}
	rec := map[string]any{
		"type":       "user",
		"uuid":       newRecordUUID(),
		"parentUuid": parent,
		"sessionId":  sessionID,
		"cwd":        cwd,
		"message":    map[string]any{"role": "user", "content": prompt},
	}
	if line, err := marshalRecord(rec); err == nil {
		appendToSession(f, line)
	}
}

// scanTranscriptTail reads the transcript at path and returns two facts the resume
// append needs: the uuid of the LAST record that carries one (the continuation's parent,
// "" when none does), and the string content of the LAST user record (to detect a
// re-entry that already wrote this prompt). Best-effort: a missing/unreadable file yields
// two empty strings, and the caller degrades to a derived parent and no dedup.
//
// Only user records with a plain STRING content are considered for lastUserContent — a
// human turn's content is a bare string, whereas a tool_result-carrying user record's
// content is a block list; comparing the latter to the prompt would never match and
// must not, so it is simply skipped here.
func scanTranscriptTail(path string) (lastUUID, lastUserContent string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec struct {
			Type    string `json:"type"`
			UUID    string `json:"uuid"`
			Message *struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if rec.UUID != "" {
			lastUUID = rec.UUID
		}
		if rec.Type == "user" && rec.Message != nil {
			var s string
			if json.Unmarshal(rec.Message.Content, &s) == nil {
				lastUserContent = s
			}
		}
	}
	return lastUUID, lastUserContent
}

// newRecordUUID returns a random RFC-4122 v4 uuid, the shape real Claude Code
// stamps on every transcript record.
//
// The format matters and not merely the uniqueness: a consumer that parses the
// field, or matches a transcript's records against a uuid it was handed
// elsewhere, is entitled to a uuid rather than an arbitrary token. On a failure
// of the random source it returns the empty string, and the caller's record
// simply carries no uuid — the pre-existing behaviour, and better than a
// predictable constant that would make two sub-agents share an identity.
func newRecordUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// appendToSession writes one JSONL line to the session file.
// Errors are silently ignored — session persistence is best-effort; the mock
// must not fail because of a session write error.
func appendToSession(f *os.File, line []byte) {
	if f == nil {
		return
	}
	f.Write(line)         //nolint:errcheck
	f.Write([]byte{'\n'}) //nolint:errcheck
}

// subagentTranscriptPath returns the path of a subagent's sidechain transcript.
//
// The <configDir>/projects/<encoded-resolved-cwd>/ prefix and the non-alphanumeric→'-'
// encoding are documented; the symlink resolution before encoding matches
// sessionFilePath's own (see resolveEncodingCwd's doc — real Claude Code resolves
// symlinks consistently, and any consumer resolving this SAME path independently — e.g.
// a10n-workspace's StableSessionID/ResolveWorkDir, driven by the sub-agent's OWN
// os.Getwd() (already resolved) once it walks back up to the parent project dir — must
// land on the identical encoded directory, or transcript resolution silently fails for
// any tool trusting a hook payload's `transcript_path` field literally). The
// <parentSessionID>/subagents/agent-<id>.jsonl suffix is NOT documented — it is
// replicated from the REAL claude CLI's on-disk layout (observed at
// ~/.claude/projects/<proj>/<root>/subagents/agent-<hash>.jsonl + .meta.json during the
// hook PoC), so the mock's transcript_path matches what real subagent hooks receive.
//
// sr:docs https://code.claude.com/docs/en/agent-sdk/sessions (projects/<encoded-cwd> prefix + CLAUDE_CONFIG_DIR)
func subagentTranscriptPath(configDir, cwd, parentSessionID, agentID string) string {
	encoded := nonAlphanumRe.ReplaceAllString(resolveEncodingCwd(cwd), "-")
	return filepath.Join(configDir, "projects", encoded, parentSessionID, "subagents", "agent-"+agentID+".jsonl")
}

// seedSubagentTranscript writes prompt as the first user record of the subagent's
// sidechain transcript and writes the .meta.json sidecar (agentType/worktreePath/
// description/toolUseId, as the real CLI does), returning the transcript path.
//
// The real Claude subagent transcript's first record IS the dispatch prompt, and
// the SubagentStart hook receives this path as transcript_path — so a hook can
// read the prompt out of it (e.g. to recover an embedded `--task-id`). Returns the
// path even on a best-effort write failure so the hook still gets a target.
//
// parentCwd keys the transcript's PROJECT DIR: real claude nests the subagent's
// sidechain under the PARENT session's transcript directory (…/projects/<encoded
// parent cwd>/<session>/subagents/…), regardless of the subagent's own worktree.
// subCwd is what the subagent's env reports (its isolated worktree under
// isolation="worktree", else == parentCwd); it is what lands in the record's `cwd`
// and the meta `worktreePath`, so a hook reading the transcript sees the isolated cwd.
//
// toolUseId is the id of the Agent/Task tool_use in the PARENT transcript that
// spawned this subagent. Real Claude Code records it in the .meta.json sidecar,
// and a consumer deriving the subagent's parentPath (which parent tool_use this
// sidechain answers) reads it — so the mock must stamp the REAL id here rather
// than the empty string a prior version hardcoded. Empty only when the spawning
// tool_use carried no id (e.g. a scenario that omitted one).
//
// sr:docs https://code.claude.com/docs/en/agent-sdk/sessions
func seedSubagentTranscript(configDir, parentCwd, subCwd, parentSessionID, agentID, agentType, toolUseID, prompt string) string {
	path := subagentTranscriptPath(configDir, parentCwd, parentSessionID, agentID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path
	}
	rec := map[string]any{
		"type":      "user",
		"sessionId": parentSessionID,
		// uuid + an explicit null parentUuid make this record the transcript's
		// ORIGIN, which is what a consumer keys the sub-agent's identity by.
		// Verified against a real Claude sub-agent transcript, whose first record
		// carries a uuid with parentUuid null and every later record chains from
		// it. The mock omitted the uuid, so every record in the file looked
		// parentless-but-anonymous and no origin could be found at all — a
		// consumer resolving a stable session id from this path got "every entry
		// has a parent" and had to stand down, which read from the outside as a
		// sub-agent whose cycle could not be judged.
		"uuid":        newRecordUUID(),
		"parentUuid":  nil,
		"isSidechain": true,
		// agentId mirrors the REAL Claude Code subagent transcript: every record
		// in subagents/agent-<AGENTID>.jsonl carries a top-level agentId equal to
		// the file's agent id. The parallel-subagent task-id attribution path
		// (locate-task-id --agent-id) reads this field, so the mock must seed it
		// for that deterministic path to be exercised in mock-based harnesses.
		"agentId": agentID,
		"cwd":     subCwd,
		"message": map[string]any{"role": "user", "content": prompt},
	}
	if line, err := marshalRecord(rec); err == nil {
		if f, ferr := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); ferr == nil {
			appendToSession(f, line)
			f.Close()
		}
	}
	// Best-effort .meta.json sidecar (mirrors the real layout; not all hooks read it).
	// worktreePath is the subagent's isolated cwd when it differs from the parent
	// (isolation="worktree"), matching the real CLI's sidecar.
	worktreePath := ""
	if subCwd != parentCwd {
		worktreePath = subCwd
	}
	meta := map[string]any{"agentType": agentType, "worktreePath": worktreePath, "description": "", "toolUseId": toolUseID}
	if mb, err := json.MarshalIndent(meta, "", "  "); err == nil {
		_ = os.WriteFile(strings.TrimSuffix(path, ".jsonl")+".meta.json", mb, 0o644)
	}
	return path
}

// openRunTranscript decides which file a run writes and which path its hooks
// are told about:
//
//   - a nested SUB-AGENT run writes the sub-agent's own sidechain file and
//     reports the PARENT's transcript_path (the sub-agent is named by agent_id);
//   - a FORK (--resume <old> --fork-session) writes a new file under this run's
//     own session id, seeded by forkTranscript;
//   - a RESUME writes the session's existing file — found in whichever project
//     directory holds it, when that is not this run's — and reports the path
//     under this run's own directory, as real Claude Code does (see transcript);
//   - anything else writes, and reports, <projects>/<encoded cwd>/<id>.jsonl.
func openRunTranscript(cfg Config) (*transcript, error) {
	stamp := recordStamp{SessionID: cfg.SessionID, Cwd: cfg.Cwd}
	if cfg.SidechainPath != "" {
		stamp.IsSidechain = true
		stamp.AgentID = cfg.AgentID
		return openTranscript(cfg.SidechainPath, cfg.ParentTranscriptPath, stamp)
	}
	here := sessionFilePath(cfg.ConfigDir, cfg.Cwd, cfg.SessionID)
	if cfg.ForkFrom != "" {
		if err := forkTranscript(cfg.ConfigDir, cfg.Cwd, cfg.ForkFrom, here, cfg.SessionID); err != nil {
			return nil, err
		}
		return openTranscript(here, here, stamp)
	}
	if cfg.IsResume {
		if _, err := os.Stat(here); err != nil {
			if found := findSessionFile(cfg.ConfigDir, cfg.SessionID); found != "" {
				return openTranscript(found, here, stamp)
			}
		}
	}
	return openTranscript(here, here, stamp)
}

// findSessionFile returns the transcript of sessionID in any project directory
// under configDir, or "" — how `claude --resume <id>` finds a session begun in
// another directory.
func findSessionFile(configDir, sessionID string) string {
	projects := filepath.Join(configDir, "projects")
	entries, err := os.ReadDir(projects)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(projects, e.Name(), sessionID+".jsonl")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// writeRootPrompt writes a FRESH session's prompt as the human turn it is,
// chained after whatever the session has written so far — which, if a
// SessionStart hook printed anything, is that hook's attachment. Its uuid is
// the deterministic `e2e-root-<session>`.
//
// A caller that pre-seeded the transcript with that same record before the
// mock ran (the older sloprail harness did) is honoured rather than
// duplicated: the record is left where it is and the preamble put ahead of it.
func writeRootPrompt(tr *transcript, sessionID, cwd, prompt string) {
	if prompt == "" {
		return
	}
	rootID := "e2e-root-" + sessionID
	if tr.exists() && fileHasUUID(tr.path, rootID) {
		seedPreamble(tr.file(), sessionID)
		return
	}
	tr.persistMap(map[string]any{
		"type":    "user",
		"uuid":    rootID,
		"cwd":     cwd,
		"message": map[string]any{"role": "user", "content": prompt},
	})
}

// appendResumePrompt writes a RESUME's prompt as the next human turn, chained
// to the last record on disk. A re-entry that already wrote this prompt as the
// last human turn is not written twice.
func appendResumePrompt(tr *transcript, sessionID, prompt string) {
	if prompt == "" {
		return
	}
	if _, last := scanTranscriptTail(tr.path); last == prompt {
		return
	}
	tr.persistMap(map[string]any{
		"type":    "user",
		"uuid":    newRecordUUID(),
		"message": map[string]any{"role": "user", "content": prompt},
	})
}

// fileHasUUID reports whether any record in path has uuid as its own.
func fileHasUUID(path, uuid string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), `"uuid":"`+uuid+`"`)
}

// forkTranscript writes dest as a FORK of the session fromID under the new
// session id newID — the shape real Claude Code leaves when it continues a
// conversation in a new transcript (`--resume <id> --fork-session`, and the
// re-fork of a resumed, compacted session).
//
// Measured on one machine: a conversation compacted once and resumed several
// times left one file per resume, EVERY one opening on a verbatim copy of the
// same compact_boundary record (same uuid, same logicalParentUuid), followed by
// copies of the records the compaction preserved — which is how each fork
// carries the boundary's logical parent after its own root — and then its own
// work. A conversation never compacted forks whole: two such real transcripts
// shared 1501 of 1585 records, origin included. So:
//
//   - with a compact_boundary in the source, the fork is the LAST boundary,
//     the records it lists as preserved, and every record after it;
//   - without one, the fork is every record of the source;
//
// with sessionId rewritten to the new id (real forks carry their own), and the
// preamble a fresh file opens with.
func forkTranscript(configDir, cwd, fromID, dest, newID string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("fork: %s already exists", dest)
	}
	src := sessionFilePath(configDir, cwd, fromID)
	if _, err := os.Stat(src); err != nil {
		src = findSessionFile(configDir, fromID)
	}
	if src == "" {
		return fmt.Errorf("fork: No conversation found with session ID: %s", fromID)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("fork: %w", err)
	}
	var recs []map[string]any
	for _, line := range strings.Split(string(data), "\n") {
		var rec map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &rec) != nil || rec == nil {
			continue
		}
		if u, _ := rec["uuid"].(string); u == "" {
			continue // bookkeeping, not part of the conversation
		}
		recs = append(recs, rec)
	}
	start := 0
	for i, rec := range recs {
		if rec["type"] == "system" && rec["subtype"] == "compact_boundary" {
			start = i
		}
	}
	segment := []map[string]any{}
	if start > 0 || (len(recs) > 0 && recs[0]["subtype"] == "compact_boundary") {
		boundary := recs[start]
		segment = append(segment, boundary)
		byUUID := map[string]map[string]any{}
		for _, rec := range recs[:start] {
			byUUID[rec["uuid"].(string)] = rec
		}
		for _, u := range preservedUUIDs(boundary) {
			if rec, ok := byUUID[u]; ok {
				segment = append(segment, rec)
			}
		}
		segment = append(segment, recs[start+1:]...)
	} else {
		segment = recs
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	var buf []byte
	for _, line := range mockPreambleRecords(newID) {
		buf = append(append(buf, line...), '\n')
	}
	for _, rec := range segment {
		rec["sessionId"] = newID
		b, err := marshalRecord(rec)
		if err != nil {
			continue
		}
		buf = append(append(buf, b...), '\n')
	}
	return os.WriteFile(dest, buf, 0o644)
}

// preservedUUIDs is the list of records a compact_boundary says the
// compaction kept, in order.
func preservedUUIDs(boundary map[string]any) []string {
	meta, _ := boundary["compactMetadata"].(map[string]any)
	pm, _ := meta["preservedMessages"].(map[string]any)
	raw, _ := pm["uuids"].([]any)
	var out []string
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// NewSessionID returns a fresh session id, the shape real Claude Code uses (a
// v4 uuid).
func NewSessionID() string { return newRecordUUID() }
