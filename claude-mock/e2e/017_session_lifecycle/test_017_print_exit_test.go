package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_35_BackgroundShellFinishingWithinTheGraceDeliversItsOutput: a `claude -p`
// run ends its background shells about five seconds after the final result, "so a
// task that finishes right after the result still delivers its output"
// (headless#background-tasks-at-exit): one that exits within the grace is completed,
// not stopped, and its output file holds what it printed and how it exited.
// sr:proves background-bash-reaped-at-exit/claude
func TestT017_35_BackgroundShellFinishingWithinTheGraceDeliversItsOutput(t *testing.T) {
	dir := t.TempDir()
	tmp := filepath.Join(dir, "tmp")
	sc := script(t, dir, "s",
		toolUse("bg", "Bash", `{"command":"sleep 1; echo GRACE-OUTPUT","description":"late finisher","run_in_background":true}`),
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"LAUNCHED @MARK@"}]}}`,
	)
	out, code := runInDir(t, dir, []string{"CLAUDE_CODE_TMPDIR=" + tmp}, "--script", sc, "--session-id", "gr-1",
		"--project-dir", dir, "--config-dir", filepath.Join(dir, "config"), "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `"status":"completed"`, "a task finishing within the grace completes")
	assert.NotContains(t, out, `"status":"stopped"`)
	outputs, err := filepath.Glob(filepath.Join(tmp, "claude-*", "*", "gr-1", "tasks", "b*.output"))
	require.NoError(t, err)
	require.Len(t, outputs, 1)
	body, err := os.ReadFile(outputs[0])
	require.NoError(t, err)
	assert.Equal(t, "GRACE-OUTPUT\n\n[exited with code 0]\n", string(body))
}

// TestT017_36_TheWaitForBackgroundAgentsEndsAtTheIdleCeiling: a `claude -p` run waits for
// its background agents, up to CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS of idle waiting
// (default ten minutes; headless#background-tasks-at-exit): past it the run stops
// waiting, drops what still runs and starts no turn for it.
// sr:proves print-waits-for-background-agents/claude
func TestT017_36_TheWaitForBackgroundAgentsEndsAtTheIdleCeiling(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sub := write(t, filepath.Join(dir, "sub.sh"), `#!/bin/sh
i=0; while [ $i -lt 60 ]; do sleep 0.1; i=$((i+1)); done
echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"SLOW-AGENT-REPLY"}]}}'
echo '{"type":"result","subtype":"success","result":"SLOW-AGENT-REPLY"}'
`, 0o755)
	sc := script(t, dir, "s",
		toolUse("ag1", "Agent", `{"prompt":"go","description":"slow agent","script":"`+sub+`","run_in_background":true}`),
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"LAUNCHED @MARK@"}]}}`,
	)
	started := time.Now()
	out, code := runInDir(t, dir, []string{"CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS=400"}, "--script", sc, "--session-id", "ce-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.Less(t, time.Since(started), 5*time.Second, "the run did not wait for the 6 s agent")
	raw, err := os.ReadFile(transcriptPath(t, cfg, dir, "ce-1"))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "task-notification", "no turn starts for the dropped agent")
	assert.False(t, strings.Contains(out, "SLOW-AGENT-REPLY"), "its partial result is dropped")

	// With no ceiling (0) the run waits for it, and its notification starts a turn.
	out, code = runInDir(t, dir, []string{"CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS=0"}, "--script", sc, "--session-id", "ce-2",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	raw, err = os.ReadFile(transcriptPath(t, cfg, dir, "ce-2"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "<task-notification>", "waited for the agent")
}
