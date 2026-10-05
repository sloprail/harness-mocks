package e2e

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_80_ASubagentScriptsRefusedCallFailsTheRun: a tool call the mock does
// not implement, asked for by a sub-agent's script, ends the whole run with the
// refusal, not only the sub-agent (adr/tool-calls-validated).
func TestT017_80_ASubagentScriptsRefusedCallFailsTheRun(t *testing.T) {
	dir := t.TempDir()
	sub := callThenReply(t, dir, "sub", "FINISHED", toolUse("x", "Teleport", `{}`))
	orch := script(t, dir, "orch", toolUse("ag1", "Agent", `{"prompt":"go","description":"refused","script":"`+sub+`"}`))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "refuse-1",
		"--project-dir", dir, "--config-dir", filepath.Join(dir, "config"), "-p", "hello")
	require.NotEqual(t, 0, code, out)
	assert.Contains(t, out, "Teleport")
}
