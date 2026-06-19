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

	path := seedSubagentTranscript(configDir, cwd, parentSessionID, agentID, agentType, prompt)
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
