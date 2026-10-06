package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_112_AGatedReceiptWaitsForTheBackgroundCommandsEnd: the scenario's gate on a background call
// ("mock_gate":{"receipt_after_end":true}, which a replay sets where the recording's event order has the
// command's task frames ahead of the call's result, runs/midturn) makes the receipt wait for the command's
// end: its frames are streamed ahead of the receipt, and its notification is handed to the agent after the
// next tool, not the launching one. Without the gate the receipt comes at once (runs/bgbash-failed).
// sr:proves task-stream-frames/claude
func TestT017_112_AGatedReceiptWaitsForTheBackgroundCommandsEnd(t *testing.T) {
	for name, tc := range map[string]struct {
		command string
		gated   bool
		want    []string
	}{
		"gated":   {"sleep 0.4; echo Q", true, []string{"task_notification", "tool_result", "tool_result"}},
		"ungated": {"sleep 0.4; echo Q", false, []string{"tool_result", "task_notification", "tool_result"}},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			log := filepath.Join(dir, "payloads.log")
			h := payloadLogger(t, dir, "log.sh", log, "")
			settings(t, dir, map[string]string{"UserPromptSubmit": h, "PreToolUse": h})
			first := toolUse("b1", "Bash", `{"command":"`+tc.command+`","run_in_background":true}`)
			if tc.gated {
				first = strings.Replace(first, `{"type":"assistant",`, `{"mock_gate":{"receipt_after_end":true},"type":"assistant",`, 1)
			}
			calls := []string{
				first,
				toolUse("b2", "Bash", `{"command":"sleep 1"}`),
			}
			out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s", calls...), "--session-id", "q-1",
				"--project-dir", dir, "--config-dir", filepath.Join(dir, "cfg"), "-p", "go")
			require.Equal(t, 0, code, out)
			var seq []string
			for _, l := range strings.Split(out, "\n") {
				var f map[string]any
				if json.Unmarshal([]byte(l), &f) != nil {
					continue
				}
				if f["subtype"] == "task_notification" {
					seq = append(seq, "task_notification")
				} else if f["type"] == "user" {
					seq = append(seq, "tool_result")
				}
			}
			assert.Equal(t, tc.want, seq, out)
			if tc.gated { // the notification reaches the agent after the second tool's PreToolUse, not before it
				var events []string
				for _, p := range payloads(t, log) {
					events = append(events, p["hook_event_name"].(string))
				}
				assert.Equal(t, []string{"UserPromptSubmit", "PreToolUse", "PreToolUse", "UserPromptSubmit"}, events)
			}
		})
	}
}
