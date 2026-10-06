package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_110_ALongForegroundBashIsATask: a foreground Bash command of the main agent that runs three
// seconds or more streams a task_started (is_backgrounded false, task_type local_bash, the call's
// description) when it crosses that, and a task_notification when it ends, both ahead of its result; a
// shorter one streams none (recorded: runs/bash-long-foreground, bash-short-foreground, hook-timeout).
// sr:proves task-stream-frames/claude
func TestT017_110_ALongForegroundBashIsATask(t *testing.T) {
	for name, tc := range map[string]struct {
		command string
		task    bool
	}{"long": {"sleep 3.3", true}, "short": {"sleep 1", false}} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			call := toolUse("b1", "Bash", `{"command":"`+tc.command+`","description":"nap"}`)
			out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s", call), "--session-id", "fg-1",
				"--project-dir", dir, "--config-dir", filepath.Join(dir, "cfg"), "-p", "go")
			require.Equal(t, 0, code, out)
			var seq []string
			for _, l := range strings.Split(out, "\n") {
				var f map[string]any
				if json.Unmarshal([]byte(l), &f) != nil {
					continue
				}
				switch {
				case f["subtype"] == "task_started":
					assert.Equal(t, false, f["is_backgrounded"])
					assert.Equal(t, "local_bash", f["task_type"])
					assert.Equal(t, "nap", f["description"])
					seq = append(seq, "started")
				case f["subtype"] == "task_notification":
					assert.Equal(t, "completed", f["status"])
					seq = append(seq, "notification")
				case f["type"] == "user":
					seq = append(seq, "result")
				}
			}
			if tc.task {
				assert.Equal(t, []string{"started", "notification", "result"}, seq)
			} else {
				assert.Equal(t, []string{"result"}, seq)
			}
		})
	}
}
