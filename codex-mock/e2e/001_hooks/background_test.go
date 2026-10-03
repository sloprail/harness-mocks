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

// The recorded run runs/bg-bash-reaped-at-exit: one command, a loop that logs a
// tick a second for 40 seconds, called with a yield of a second so that it keeps
// running after the call returns; the model then ends its turn at once.

// backgroundScript calls Bash once, with $CALLS' first line as the command and
// a yield time, then ends the turn.
const backgroundScript = `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
if [ "$n" = 0 ]; then
  printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_0","name":"Bash","input":{"command":%s,"yield_time_ms":1000}}]}}\n' "$(sed -n 1p "$CALLS" | jq -Rs .)"
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"LAUNCHED"}]}}' '{"type":"result","subtype":"success","result":"LAUNCHED"}'
`

func ticks(text string) int { return strings.Count(text, "BackgroundTick") }

// A command still running when the run's other work is done is terminated: the
// run ends with the turn, without waiting for the command, which logs no more
// after it. The stream shows the command started and never ended, and the
// agent's turn and the Stop hook go as for any other (runs/bg-bash-reaped-at-exit).
// sr:proves background-bash-reaped-at-exit/codex
func TestBackgroundCommandIsTerminatedAtExit(t *testing.T) {
	rec := loadRecording(t, "bg-bash-reaped-at-exit")
	prompt := strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt")))
	cmd := prompt[strings.Index(prompt, "The command: ")+len("The command: "):]

	recLog := readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))
	require.Positive(t, ticks(recLog))
	require.Less(t, ticks(recLog), 40, "the recorded command did not run to its end")
	recStream := result{Stdout: readFile(t, filepath.Join(rec.sample, "stream.jsonl"))}

	start := time.Now()
	got := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:    backgroundScript,
		Prompt:    prompt,
		Env:       withCalls(t, cmd),
	})
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.Less(t, time.Since(start), 30*time.Second, "the run did not wait for the command")

	// the stream is the recorded one: the command started, and no end for it
	assert.Equal(t, streamShape(recStream.stream()), streamShape(got.stream()))
	gotCmds, _ := got.commands()
	assert.Empty(t, gotCmds, "no command completed")

	logPath := filepath.Join(got.Tmp, "hook.log")
	n := ticks(readFile(t, logPath))
	assert.Positive(t, n, "the command ran while the turn went on")
	assert.Less(t, n, 40)
	time.Sleep(2500 * time.Millisecond)
	b, _ := os.ReadFile(logPath)
	assert.Equal(t, n, ticks(string(b)), "the terminated command logs no more")

	var stops int
	for _, l := range got.hookLog() {
		if l["hook_event_name"] == "Stop" {
			stops++
		}
	}
	assert.Equal(t, 1, stops)
}
