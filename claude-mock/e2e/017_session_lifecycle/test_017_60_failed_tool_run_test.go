package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_60_AFailingToolCallDoesNotFailTheRun: a Bash command that exits 3 is
// the tool's failure, not the run's: PostToolUseFailure fires with the error
// the agent got, the run still exits 0, and the stream carries exactly one
// result, a success (recorded: snapshots/runs/bashfail; docs, headless: exit 0
// on success).
// sr:proves noninteractive-run/claude
func TestT017_60_AFailingToolCallDoesNotFailTheRun(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"PostToolUse": h, "PostToolUseFailure": h})
	root := script(t, dir, "root", toolUse("f", "Bash", `{"command":"echo out; exit 3"}`))
	out, code := runInDir(t, dir, nil, "--script", root, "--session-id", "fail-1", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, "the run succeeds: %s", out)

	var failures, successes int
	for _, p := range payloads(t, log) {
		switch p["hook_event_name"] {
		case "PostToolUseFailure":
			failures++
			assert.Equal(t, "Bash", p["tool_name"])
			assert.Equal(t, "Exit code 3\nout", p["error"])
		case "PostToolUse":
			successes++
		}
	}
	assert.Equal(t, 1, failures, "PostToolUseFailure fires once")
	assert.Zero(t, successes, "and no PostToolUse for the failed call")

	results := 0
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, `"type":"result"`) {
			results++
			assert.Contains(t, l, `"subtype":"success"`)
			assert.NotContains(t, l, `"is_error":true`)
		}
	}
	assert.Equal(t, 1, results, "exactly one result frame")
}
