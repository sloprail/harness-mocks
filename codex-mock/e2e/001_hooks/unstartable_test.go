package e2e

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A hook whose command cannot be launched (command not found, exit 127) is a
// non-blocking failure: the tool call it guards still runs and the turn goes
// on (runs/hook-unstartable).
// sr:proves hook-exit-code-semantics/codex
func TestUnstartableHookDoesNotBlockTheCall(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	r := execMock(t, scenario{
		HooksJSON: hooksJSON("/nonexistent/codex-mock-no-such-hook", "PreToolUse"),
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "touch "+marker),
	})
	assert.Equal(t, 0, r.Code, r.Stderr)
	assert.FileExists(t, marker, "an unstartable hook refused the call")
	assert.Contains(t, r.Stdout, `"text":"DONE"`, "the turn did not go on")
}
