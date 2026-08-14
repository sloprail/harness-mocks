package runner

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSeedSubagentTranscriptCarriesAgentId verifies the seeded first user record
// of a subagent's sidechain transcript carries a top-level agentId equal to the
// passed agentID. This mirrors the REAL Claude Code subagent transcript layout
// and is what the parallel-subagent task-id attribution path
// (locate-task-id --agent-id) reads, so mock-based harnesses must exercise it.
func TestSeedSubagentTranscriptCarriesAgentId(t *testing.T) {
	configDir := t.TempDir()
	const (
		cwd             = "/some/work/dir"
		parentSessionID = "parent-session-xyz"
		agentID         = "agent-abc123"
		agentType       = "general-purpose"
		prompt          = "do the thing --task-id T42"
	)

	// Non-worktree subagent: subCwd == parentCwd (both = cwd).
	path := seedSubagentTranscript(configDir, cwd, cwd, parentSessionID, agentID, agentType, prompt)
	require.NotEmpty(t, path)
	require.Equal(t, path, subagentTranscriptPath(configDir, cwd, parentSessionID, agentID))

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

	path := seedSubagentTranscript(configDir, cwd, cwd, "parent-session", "agent-1", "general-purpose", "do it")

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
