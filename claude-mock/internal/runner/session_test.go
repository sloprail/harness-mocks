package runner

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSeedSubagentTranscriptCarriesAgentId verifies the seeded first user record
// of a subagent's sidechain transcript carries a top-level agentId equal to the
// passed agentID. This mirrors the REAL Claude Code subagent transcript layout
// and is what the parallel-subagent task-id attribution path
// (locate-task-id --agent-id) reads, so mock-based harnesses must exercise it.
// staged:proves subagent-transcripts/claude
func TestSeedSubagentTranscriptCarriesAgentId(t *testing.T) {
	configDir := t.TempDir()
	const (
		cwd             = "/some/work/dir"
		parentSessionID = "parent-session-xyz"
		agentID         = "agent-abc123"
		agentType       = "general-purpose"
		toolUseID       = "toolu_parent_task_1"
		prompt          = "do the thing --task-id T42"
	)

	// Non-worktree subagent: subCwd == parentCwd (both = cwd).
	path := filepath.Join(configDir, parentSessionID, "subagents", "agent-"+agentID+".jsonl")
	seedSubagentTranscript(path, cwd, parentSessionID, agentID, prompt, subagentMeta{AgentType: agentType, ToolUseID: toolUseID, Description: "the thing", SpawnDepth: 1, RequestShape: "foreground", RequestNonInteractive: true})

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	sc := bufio.NewScanner(f)
	require.True(t, sc.Scan(), "transcript must have at least one record")

	var rec map[string]any
	require.NoError(t, json.Unmarshal(sc.Bytes(), &rec))

	assert.Equal(t, agentID, rec["agentId"], "seeded record must carry agentId == passed agentID")
	assert.Equal(t, true, rec["isSidechain"])
	assert.Equal(t, parentSessionID, rec["sessionId"])
	assert.Equal(t, "user", rec["type"])

	// The .meta.json sidecar must record the SPAWNING tool_use's id as toolUseId,
	// matching real Claude Code — a consumer deriving the subagent's parentPath
	// reads this field, so a prior hardcoded "" broke that derivation.
	metaData, err := os.ReadFile(strings.TrimSuffix(path, ".jsonl") + ".meta.json")
	require.NoError(t, err, ".meta.json sidecar must be written")
	var meta map[string]any
	require.NoError(t, json.Unmarshal(metaData, &meta))
	assert.Equal(t, toolUseID, meta["toolUseId"], "meta.json toolUseId must be the spawning tool_use id")
	assert.Equal(t, agentType, meta["agentType"])
}

// TestSeedSubagentTranscript_SeedsAnOriginRecord: the seeded record must be an
// ORIGIN — a uuid, and parentUuid explicitly null.
//
// This is how a consumer keys a sub-agent's identity: it scans for the first
// record with a uuid and no parent, and that record's uuid IS the conversation's
// stable id. Verified against a real Claude sub-agent transcript, whose first
// record carries a uuid with parentUuid null and whose every later record chains
// from it.
//
// The mock seeded neither field. Every record in the file was therefore
// anonymous, no origin could be found, and a consumer resolving an identity from
// the path was told "every entry has a parent" and had to stand down — so a
// sub-agent's cycle went unjudged for a reason that looked like a missing
// harness capability rather than a missing field.
func TestSeedSubagentTranscript_SeedsAnOriginRecord(t *testing.T) {
	configDir := t.TempDir()
	const cwd = "/some/work/dir"

	path := filepath.Join(configDir, "parent-session", "subagents", "agent-agent-1.jsonl")
	seedSubagentTranscript(path, cwd, "parent-session", "agent-1", "do it", subagentMeta{AgentType: "general-purpose", ToolUseID: "toolu_parent_task_1"})

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	sc := bufio.NewScanner(f)
	require.True(t, sc.Scan(), "transcript must have at least one record")
	var rec map[string]any
	require.NoError(t, json.Unmarshal(sc.Bytes(), &rec))

	uuid, _ := rec["uuid"].(string)
	assert.NotEmpty(t, uuid,
		"the seeded record must carry a uuid — without one it is not an origin and no identity can be resolved from this transcript")
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, uuid,
		"the uuid must have the RFC-4122 v4 shape real Claude Code writes")

	parent, present := rec["parentUuid"]
	assert.True(t, present, "parentUuid must be PRESENT and null, as the real origin record has it")
	assert.Nil(t, parent, "the origin record must have no parent")
}

// TestNewRecordUUID_IsUniquePerCall: two sub-agents must not share an identity.
//
// The uuid IS the stable session id a consumer derives, so a constant here would
// silently key two sub-agents' state to one place — the exact confusion the
// sidechain layout exists to prevent.
func TestNewRecordUUID_IsUniquePerCall(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		u := newRecordUUID()
		require.NotEmpty(t, u)
		require.False(t, seen[u], "newRecordUUID repeated %s", u)
		seen[u] = true
	}
}

// openTmpSession opens (creating) a scratch session JSONL file for the append tests
// and registers its cleanup.
func openTmpSession(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(t.TempDir(), "session.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	return f
}

// readRecords parses every JSONL line of f's file back into maps.
func readRecords(t *testing.T, f *os.File) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(f.Name())
	require.NoError(t, err)
	var recs []map[string]any
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		recs = append(recs, rec)
	}
	return recs
}

// TestAppendResumePromptTranscript_WritesAContinuationHumanRecord: a resume append
// writes the prompt as a user record with its OWN uuid and a NON-null parentUuid —
// a mid-conversation human turn, not a second parentless root. This is the shape a
// reader (sr-session's cite/normalize) needs to treat it as a follow-up human message.
func TestAppendResumePromptTranscript_WritesAContinuationHumanRecord(t *testing.T) {
	f := openTmpSession(t)
	const (
		sessionID = "s-resume-1"
		cwd       = "/some/work/dir"
	)
	// Seed the root first, exactly as a fresh run would: the transcript already has
	// its origin when a resume appends.
	seedRootPromptTranscript(f, sessionID, cwd, "the first human prompt")
	appendResumePromptTranscript(f, sessionID, cwd, "the second human prompt")

	recs := readRecords(t, f)
	require.Len(t, recs, 2, "root + one appended continuation")

	root, cont := recs[0], recs[1]
	assert.Equal(t, "the first human prompt", msgContent(root), "root carries the first prompt")
	// The root stays a parentless origin — it must NOT have grown a parent.
	if p, present := root["parentUuid"]; present {
		assert.Nil(t, p, "the root must remain parentless")
	}

	assert.Equal(t, "user", cont["type"])
	assert.Equal(t, "the second human prompt", msgContent(cont), "the continuation carries the second prompt")
	uuid, _ := cont["uuid"].(string)
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, uuid,
		"the continuation must carry a fresh RFC-4122 v4 uuid, or a reader skips it as a no-uuid preamble line")
	parent, present := cont["parentUuid"]
	assert.True(t, present, "parentUuid must be present")
	assert.NotNil(t, parent, "the continuation's parentUuid must be NON-null — a null parent would read as a second root")
	assert.NotEmpty(t, parent, "the continuation's parentUuid must be non-empty")
	assert.Equal(t, sessionID, cont["sessionId"])
	assert.Equal(t, cwd, cont["cwd"])
}

// TestAppendResumePromptTranscript_TwoResumesTwoDistinctHumanRecords: two resume
// appends produce TWO distinct continuation human records, each with its own uuid —
// the multi-human-turn transcript two same-session runs must build.
func TestAppendResumePromptTranscript_TwoResumesTwoDistinctHumanRecords(t *testing.T) {
	f := openTmpSession(t)
	const (
		sessionID = "s-resume-2"
		cwd       = "/some/work/dir"
	)
	seedRootPromptTranscript(f, sessionID, cwd, "root prompt")
	appendResumePromptTranscript(f, sessionID, cwd, "second human turn")
	appendResumePromptTranscript(f, sessionID, cwd, "third human turn")

	recs := readRecords(t, f)
	require.Len(t, recs, 3, "root + two distinct continuations")
	assert.Equal(t, "second human turn", msgContent(recs[1]))
	assert.Equal(t, "third human turn", msgContent(recs[2]))

	u1, _ := recs[1]["uuid"].(string)
	u2, _ := recs[2]["uuid"].(string)
	assert.NotEmpty(t, u1)
	assert.NotEmpty(t, u2)
	assert.NotEqual(t, u1, u2, "each continuation must carry its own distinct uuid")
	// Both continuations are non-null-parented human turns.
	for _, r := range recs[1:] {
		assert.NotNil(t, r["parentUuid"])
		assert.NotEmpty(t, r["parentUuid"])
	}
}

// TestAppendResumePromptTranscript_ReEntryDoesNotDuplicate: the append fires ONCE per
// resume — a second call with the SAME prompt (the re-entry signature) is a no-op, so
// re-opening a resumed transcript does not double-write the human turn.
func TestAppendResumePromptTranscript_ReEntryDoesNotDuplicate(t *testing.T) {
	f := openTmpSession(t)
	const (
		sessionID = "s-resume-3"
		cwd       = "/some/work/dir"
	)
	seedRootPromptTranscript(f, sessionID, cwd, "root prompt")
	appendResumePromptTranscript(f, sessionID, cwd, "the resume prompt")
	appendResumePromptTranscript(f, sessionID, cwd, "the resume prompt") // re-entry: identical prompt

	recs := readRecords(t, f)
	require.Len(t, recs, 2, "the identical re-entry prompt must NOT be written twice")
	assert.Equal(t, "the resume prompt", msgContent(recs[1]))
}

// TestAppendResumePromptTranscript_ChainsToLastUUIDWhenPresent: when the transcript's
// last record carries a uuid, the continuation chains to THAT uuid — a real
// continuation link, not just any non-null placeholder.
func TestAppendResumePromptTranscript_ChainsToLastUUIDWhenPresent(t *testing.T) {
	f := openTmpSession(t)
	const (
		sessionID = "s-resume-4"
		cwd       = "/some/work/dir"
	)
	// Two uuid-bearing records already on disk; the append must chain from the LAST one.
	rootUUID := newRecordUUID()
	lastUUID := newRecordUUID()
	appendToSession(f, mustJSON(t, map[string]any{"type": "user", "uuid": rootUUID, "parentUuid": nil, "message": map[string]any{"role": "user", "content": "root"}}))
	appendToSession(f, mustJSON(t, map[string]any{"type": "assistant", "uuid": lastUUID, "parentUuid": rootUUID, "message": map[string]any{"role": "assistant", "content": "ok"}}))

	appendResumePromptTranscript(f, sessionID, cwd, "next human turn")

	recs := readRecords(t, f)
	require.Len(t, recs, 3)
	assert.Equal(t, lastUUID, recs[2]["parentUuid"], "the continuation must chain to the transcript's last uuid")
}

// msgContent pulls the string content out of a record's message.content, or "" when
// it is not a plain string (a tool_result block list, etc.).
func msgContent(rec map[string]any) string {
	msg, ok := rec["message"].(map[string]any)
	if !ok {
		return ""
	}
	s, _ := msg["content"].(string)
	return s
}

// mustJSON marshals v or fails the test.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
