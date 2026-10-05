package e2e

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// At the idle ceiling the run stops what still runs (headless#background-tasks-at-exit):
// the background agent's own process is gone, so the marker it would write once
// its sleep ends is never written.
// sr:proves print-waits-for-background-agents/claude
func TestT017_91_TheIdleCeilingStopsTheAgentsProcess(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	sub := write(t, filepath.Join(dir, "sub.sh"), `#!/bin/sh
sleep 2
touch `+marker+`
echo '{"type":"result","subtype":"success","result":"LATE"}'
`, 0o755)
	sc := script(t, dir, "s",
		toolUse("ag1", "Agent", `{"prompt":"go","description":"late agent","script":"`+sub+`","run_in_background":true}`),
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"LAUNCHED @MARK@"}]}}`,
	)
	out, code := runInDir(t, dir, []string{"CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS=300"}, "--script", sc, "--session-id", "ce-stop",
		"--project-dir", dir, "--config-dir", filepath.Join(dir, "config"), "-p", "hello")
	require.Equal(t, 0, code, out)
	time.Sleep(3 * time.Second) // past the moment the agent's sleep would have ended
	assert.NoFileExists(t, marker, "the agent was stopped at the ceiling, not left running")
}
