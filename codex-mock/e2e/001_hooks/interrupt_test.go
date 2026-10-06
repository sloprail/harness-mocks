package e2e

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// interruptingWriter is a run's stdout that sends the run SIGINT when a command has started.
type interruptingWriter struct {
	out  *bytes.Buffer
	cmd  *exec.Cmd
	once sync.Once
}

func (w *interruptingWriter) Write(p []byte) (int, error) {
	n, err := w.out.Write(p)
	if strings.Contains(string(p), `"command_execution"`) && strings.Contains(string(p), `"in_progress"`) {
		w.once.Do(func() { _ = w.cmd.Process.Signal(syscall.SIGINT) })
	}
	return n, err
}

// interruptShape is, in order, what a rollout holds of an interrupted turn: the call, what the
// agent was told of it (the time masked), what it was told of the interruption, and the turn's
// end as aborted.
func interruptShape(t *testing.T, rollout string) (out []string) {
	wall := regexp.MustCompile(`after [0-9.]+s`)
	for _, l := range jsonLines(rollout) {
		p, _ := l["payload"].(map[string]any)
		switch {
		case p["type"] == "function_call" || p["type"] == "custom_tool_call":
			out = append(out, "call")
		case strings.HasSuffix(fmt.Sprint(p["type"]), "_call_output"):
			out = append(out, wall.ReplaceAllString(textOf(p["output"]), "after <T>s"))
		case p["type"] == "message" && p["role"] == "user" && strings.Contains(fmt.Sprint(p["content"]), "turn_aborted"):
			out = append(out, "user: turn_aborted")
		case p["type"] == "turn_aborted":
			out = append(out, "event: turn_aborted "+fmt.Sprint(p["reason"]))
		}
	}
	return out
}

// textOf is the text of a tool output, a string or a list of text parts.
func textOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	var b strings.Builder
	parts, _ := v.([]any)
	for _, part := range parts {
		m, _ := part.(map[string]any)
		s, _ := m["text"].(string)
		b.WriteString(s)
	}
	return b.String()
}

// A user's Ctrl-C (SIGINT) while the agent's command runs interrupts the turn: the Interrupt hook
// fires (its matcher, like the prompt's and the stop's, is ignored), the command is stopped and
// reported to the agent as "aborted by user after <time>", the agent is told the user interrupted,
// the turn is recorded as aborted, the stream prints nothing more, no PostToolUse and no Stop hook
// runs, the session still ends, and the run exits 1 (recorded: runs/interrupt-hook).
// sr:proves hook-matcher-filter/codex
func TestAnInterruptedTurnFiresTheInterruptHookAndIsAborted(t *testing.T) {
	rec := loadRecording(t, "interrupt-hook")
	recorded := func(event string) []map[string]any {
		var out []map[string]any
		for _, l := range jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))) {
			if l["hook_event_name"] == event {
				out = append(out, l)
			}
		}
		return out
	}
	assert.Len(t, recorded("Interrupt"), 1)
	assert.Empty(t, recorded("PostToolUse"))
	assert.Empty(t, recorded("Stop"))
	assert.Len(t, recorded("SessionEnd"), 1)
	assert.Equal(t, "1\n", readFile(t, filepath.Join(rec.sample, "exit.txt")))

	got := execMock(t, scenario{
		HooksJSON: `{"hooks":{"Interrupt":[{"matcher":"never","hooks":[{"type":"command","command":"cat >>\"$HOOK_LOG\"; echo >>\"$HOOK_LOG\""}]}],` +
			`"PostToolUse":[{"hooks":[{"type":"command","command":"cat >>\"$HOOK_LOG\"; echo >>\"$HOOK_LOG\""}]}],` +
			`"Stop":[{"hooks":[{"type":"command","command":"cat >>\"$HOOK_LOG\"; echo >>\"$HOOK_LOG\""}]}],` +
			`"SessionEnd":[{"hooks":[{"type":"command","command":"cat >>\"$HOOK_LOG\"; echo >>\"$HOOK_LOG\""}]}]}}`,
		Script: callThenResult, Prompt: "go", Env: withCalls(t, "sleep 30"), InterruptOnCommand: true,
	})
	assert.Equal(t, 1, got.Code, "an interrupted run exits 1")
	var names []string
	for _, l := range got.hookLog() {
		names = append(names, l["hook_event_name"].(string))
	}
	assert.Equal(t, []string{"Interrupt", "SessionEnd"}, names, "the matcher of Interrupt is ignored; no PostToolUse, no Stop")
	assert.NotContains(t, got.Stdout, `"turn.completed"`)
	assert.NotContains(t, got.Stdout, `"status":"completed"`, "the interrupted command is not reported as ended")
	want := []string{"call", "aborted by user after <T>s", "user: turn_aborted", "event: turn_aborted interrupted"}
	assert.Equal(t, want, interruptShape(t, recordedRollout(t, rec)), "recorded")
	assert.Equal(t, want, interruptShape(t, got.rollout(t)), "the mock's")
}

// What an Interrupt hook answers cannot prevent the interruption or be shown: hooks that print a
// systemMessage as JSON, plain text, or exit 2 with a reason all run, and neither the stream, the
// transcript nor the run's error output carries what they said; the turn is aborted and the run exits
// 1 all the same (recorded: runs/interrupt-hook-output, the doc's hooks#interrupt).
// sr:proves hook-matcher-filter/codex
func TestWhatAnInterruptHookAnswersCannotPreventTheInterruption(t *testing.T) {
	rec := loadRecording(t, "interrupt-hook-output")
	ran := func(log []map[string]any) (out []string) {
		for _, l := range log {
			if r, ok := l["ran"].(string); ok {
				out = append(out, r)
			}
		}
		sort.Strings(out)
		return
	}
	want := []string{"exit2", "json", "none", "plain"}
	recordedFiles := []string{"stream.jsonl", "stderr.txt"}
	assert.Equal(t, want, ran(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl")))), "recorded")
	assert.Equal(t, "1\n", readFile(t, filepath.Join(rec.sample, "exit.txt")))
	for _, f := range recordedFiles {
		assert.NotContains(t, readFile(t, filepath.Join(rec.sample, f)), "INT-", f)
	}
	assert.NotContains(t, recordedRollout(t, rec), "INT-")

	got := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "sleep 30"), InterruptOnCommand: true,
	})
	assert.Equal(t, 1, got.Code)
	assert.Equal(t, want, ran(got.hookLog()), "the mock's")
	assert.NotContains(t, got.Stdout, "INT-")
	assert.NotContains(t, got.Stderr, "INT-")
	assert.NotContains(t, got.rollout(t), "INT-")
}

// An Interrupt hook is not cut short by the SessionEnd limit of one second by default and three at
// most: of three hooks waiting 2 seconds with no timeout set, 2 under a configured 3, and 5 under a
// configured 10, all finish, and the run waits for them (recorded: runs/interrupt-hook-timeout, which
// differs from the docs' "Interrupt use 1 second by default and support up to 3"). The mock gives
// Interrupt hooks the ordinary default and no cap.
// sr:proves hook-timeout/codex
func TestInterruptHooksAreNotCutShortByTheSessionEndLimit(t *testing.T) {
	rec := loadRecording(t, "interrupt-hook-timeout")
	done := func(log []map[string]any) (out []string) {
		for _, l := range log {
			if d, ok := l["done"].(string); ok {
				out = append(out, d)
			}
		}
		sort.Strings(out)
		return
	}
	want := []string{"capped", "default", "limit3"}
	assert.Equal(t, want, done(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl")))), "recorded")
	got := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "sleep 30"), InterruptOnCommand: true,
	})
	assert.Equal(t, 1, got.Code)
	assert.Equal(t, want, done(got.hookLog()), "the mock's")
}

// An Interrupt hook whose timeout is over the 3 seconds Interrupt hooks may have is clamped, and Codex
// warns once per such hook, naming the hooks file; hooks with none set, or 3 or less, draw no warning
// (recorded: runs/interrupt-hook-timeout, whose hook of 10 seconds drew the one warning).
// sr:proves hook-timeout/codex
func TestAnInterruptHookTimeoutOverThreeSecondsIsClampedWithAWarning(t *testing.T) {
	clamps := func(stream string) (n int) {
		for _, e := range jsonLines(stream) {
			if item, _ := e["item"].(map[string]any); item["type"] == "error" && strings.Contains(fmt.Sprint(item["message"]), "clamping Interrupt hook timeout to 3s in ") {
				n++
			}
		}
		return n
	}
	rec := loadRecording(t, "interrupt-hook-timeout")
	assert.Equal(t, 1, clamps(readFile(t, filepath.Join(rec.sample, "stream.jsonl"))), "recorded: one warning, for the hook of 10")
	for name, tc := range map[string]struct {
		timeouts string
		want     int
	}{"none and 3": {`{"type":"command","command":"true"},{"type":"command","command":"true","timeout":3}`, 0},
		"10":          {`{"type":"command","command":"true","timeout":10}`, 1},
		"10, 4 and 1": {`{"type":"command","command":"true","timeout":10},{"type":"command","command":"true","timeout":4},{"type":"command","command":"true","timeout":1}`, 2}} {
		t.Run(name, func(t *testing.T) {
			got := execMock(t, scenario{HooksJSON: `{"hooks":{"Interrupt":[{"hooks":[` + tc.timeouts + `]}]}}`, Script: callThenResult, Prompt: "go", Env: withCalls(t, "true"), BypassTrust: true})
			require.Equal(t, 0, got.Code, got.Stderr)
			assert.Equal(t, tc.want, clamps(got.Stdout))
		})
	}
}
