package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_85_SubagentCompactionIsLoggedInItsOwnFile: a sub-agent that compacts
// its own context leaves the compact_boundary and its summary in its own
// subagents/agent-<id>.jsonl, under its own chain and sidechain flags, and the
// session's file holds no boundary (sub-agents doc, auto-compaction).
// sr:proves subagent-transcripts/claude
func TestT017_85_SubagentCompactionIsLoggedInItsOwnFile(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sub := script(t, dir, "sub",
		toolUse("sb1", "Bash", `{"command":"true"}`),
		`{"type":"compact","summary":"sub summary @MARK@","trigger":"auto","pre_tokens":9000,"post_tokens":900}`)
	orch := script(t, dir, "orch", toolUse("ag1", "Agent", `{"prompt":"work","description":"compacter","script":"`+sub+`"}`))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "subcmp-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	main := transcriptPath(t, cfg, dir, "subcmp-1")
	assert.NotContains(t, readString(t, main), "compact_boundary", "the session's own file has no boundary")
	sides, err := filepath.Glob(filepath.Join(strings.TrimSuffix(main, ".jsonl"), "subagents", "agent-*.jsonl"))
	require.NoError(t, err)
	require.Len(t, sides, 1)
	agentID := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(sides[0]), "agent-"), ".jsonl")
	boundaries := 0
	for _, r := range readRecs(t, sides[0]) {
		if r.Subtype == "compact_boundary" {
			boundaries++
			var full map[string]any
			require.NoError(t, json.Unmarshal([]byte(r.Raw), &full))
			assert.Equal(t, "system", full["type"])
			meta := full["compactMetadata"].(map[string]any)
			assert.Equal(t, "auto", meta["trigger"])
			assert.EqualValues(t, 9000, meta["preTokens"])
			assert.True(t, r.IsSidechain)
			assert.Equal(t, agentID, r.AgentID)
			assert.Equal(t, "subcmp-1", r.SessionID)
		}
	}
	assert.Equal(t, 1, boundaries, "the boundary is in the sub-agent's file")
	assert.Contains(t, readString(t, sides[0]), `"isCompactSummary":true`)
	assert.NotContains(t, readString(t, main), "sub summary")
}
