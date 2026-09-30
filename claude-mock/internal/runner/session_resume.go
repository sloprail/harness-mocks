package runner

import (
	"encoding/json"
	"os"
	"strings"
)

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
