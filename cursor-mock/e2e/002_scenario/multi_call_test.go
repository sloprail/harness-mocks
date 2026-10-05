package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/task-stream-frames: a turn with a shell call and a Task
// call has both started before either is completed, in the order the calls were
// made: shell started, Task started, shell completed, Task completed.
func TestTheCallsOfOneTurnAreAllStartedBeforeAnyIsCompleted(t *testing.T) {
	sub := filepath.Join(t.TempDir(), "sub.sh")
	require.NoError(t, os.WriteFile(sub, []byte(`#!/bin/sh
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"PONG"}]}}'
`), 0o755))
	shell := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu_1","name":"Bash","input":{"command":"echo hi"}}]}}`
	task := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu_2","name":"Task","input":{"description":"Reply PONG","prompt":"Reply PONG","script":"` + sub + `"}}]}}`
	out, stderr, code := run(t, `#!/bin/sh
if grep -q tool_use "$A10N_MOCK_SESSION_FILE"; then
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"DONE"}]}}' '`+done+`'
else
  printf '%s\n' '`+shell+`' '`+task+`'
fi
`, "two calls")
	require.Equal(t, 0, code, stderr)

	var order []string
	for _, f := range frames(out) {
		if call, ok := f["tool_call"].(map[string]any); ok {
			kind := "shell"
			if call["taskToolCall"] != nil {
				kind = "task"
			}
			order = append(order, kind+" "+f["subtype"].(string))
		}
	}
	assert.Equal(t, []string{"shell started", "task started", "shell completed", "task completed"}, order)
}
