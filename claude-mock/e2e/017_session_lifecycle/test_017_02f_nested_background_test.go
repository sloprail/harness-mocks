package e2e

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_02f_LauncherDoesNotWaitForNestedBackground: in a `-p` session a
// sub-agent that launched a background sub-agent finishes without waiting for
// it: its result reaches the main conversation (the PostToolUse of the main
// thread's Agent call) while the nested one is still running, and only then is
// the nested one let go. A launcher that waited would never get there.
// sr:proves background-agent/claude
// sr:proves nested-subagents/claude
func TestT017_02f_LauncherDoesNotWaitForNestedBackground(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, `case "$IN" in *'"hook_event_name":"PostToolUse"'*'"description":"outer"'*) touch `+dir+`/released;; esac`)
	settings(t, dir, map[string]string{"PostToolUse": h})
	inner := write(t, filepath.Join(dir, "inner.sh"), `#!/bin/sh
while [ ! -f `+dir+`/released ]; do sleep 0.05; done
echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"INNER-DONE"}]}}'
echo '{"type":"result","subtype":"success","result":"INNER-DONE"}'
`, 0o755)
	outer := script(t, dir, "outer", toolUse("out1", "Agent", `{"prompt":"deep","description":"inner","script":"`+inner+`","run_in_background":true}`))
	root := script(t, dir, "root", toolUse("r1", "Agent", `{"prompt":"layer","description":"outer","script":"`+outer+`"}`))
	out, code := runInDir(t, dir, nil, "--script", root, "--session-id", "nest-bg",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.FileExists(t, filepath.Join(dir, "released"))
}
