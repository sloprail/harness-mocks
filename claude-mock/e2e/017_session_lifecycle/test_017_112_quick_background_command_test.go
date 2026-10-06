package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_112_ABackgroundCommandThatEndsAtOnceIsReportedAheadOfItsReceipt: a background command that has
// ended by the time its receipt is written (an echo) streams its task frames ahead of the receipt, as the
// recorded midturn run does, and its notification is handed to the agent after the next tool, not the
// launching one; one that is still running (a sleep of 0.4 s) is reported after the receipt
// (recorded: runs/midturn, bgbash-failed).
// sr:proves task-stream-frames/claude
func TestT017_112_ABackgroundCommandThatEndsAtOnceIsReportedAheadOfItsReceipt(t *testing.T) {
	for name, tc := range map[string]struct {
		command string
		want    []string
	}{
		"quick": {"echo Q", []string{"task_notification", "tool_result", "tool_result"}},
		"slow":  {"sleep 0.4; echo Q", []string{"tool_result", "task_notification", "tool_result"}},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			log := filepath.Join(dir, "payloads.log")
			h := payloadLogger(t, dir, "log.sh", log, "")
			settings(t, dir, map[string]string{"UserPromptSubmit": h, "PreToolUse": h})
			calls := []string{
				toolUse("b1", "Bash", `{"command":"`+tc.command+`","run_in_background":true}`),
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
			if name == "quick" { // the notification reaches the agent after the second tool's PreToolUse, not before it
				var events []string
				for _, p := range payloads(t, log) {
					events = append(events, p["hook_event_name"].(string))
				}
				assert.Equal(t, []string{"UserPromptSubmit", "PreToolUse", "PreToolUse", "UserPromptSubmit"}, events)
			}
		})
	}
}
