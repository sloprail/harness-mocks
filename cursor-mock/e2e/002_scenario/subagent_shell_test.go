package e2e

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/foreground-subagent-bash-ends-with-response: a
// foreground sub-agent (the Task tool) starts a shell command without waiting
// for it (block_until_ms 0) and answers; the command is gone once the sub-agent
// has answered, and the stream says it was aborted.

func jsonLines(t *testing.T, path string) (out []map[string]any) {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var m map[string]any
		if json.Unmarshal(sc.Bytes(), &m) == nil && m != nil {
			out = append(out, m)
		}
	}
	return out
}

// recorded is the run's newest sample, and its setup directory.
func recorded(t *testing.T) (sample, setup string) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "snapshots", "runs", "foreground-subagent-bash-ends-with-response")
	samples, err := filepath.Glob(filepath.Join(root, "samples", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, samples)
	sort.Strings(samples)
	return samples[len(samples)-1], filepath.Join(root, "setup")
}

// backgroundOf is, of a run's hook payloads, the events of the Shell call that
// ran command, in order, and the postToolUse receipt.
func backgroundOf(payloads []map[string]any, command string) (events []string, receipt string, session string) {
	for _, p := range payloads {
		in, _ := p["tool_input"].(map[string]any)
		cmd, _ := p["command"].(string)
		if in["command"] != command && cmd != command {
			continue
		}
		name, _ := p["hook_event_name"].(string)
		events = append(events, name)
		if name == "postToolUse" {
			receipt, _ = p["tool_output"].(string)
			session, _ = p["session_id"].(string)
		}
	}
	return events, receipt, session
}

// notifications are the task_notification frames of a stream.
func notifications(frames []map[string]any) (out []map[string]any) {
	for _, f := range frames {
		if f["type"] == "system" && f["subtype"] == "task_notification" {
			out = append(out, f)
		}
	}
	return out
}

// TestACommandAForegroundSubagentStartedInTheBackgroundEndsWithItsResponse:
// recorded, a command a foreground sub-agent starts without waiting for it is
// gone once the sub-agent has given its final response, and the stream reports
// it as an aborted task_notification of the run's own session. The sub-agent's
// own calls fire the hooks under its own session id, are not on the stream, and
// give the receipt {shell_id, pid} with no afterShellExecution; the receipt
// does not say the command will be terminated (declared in the cell).
// sr:proves foreground-subagent-bash-ends-with-response/cursor
func TestACommandAForegroundSubagentStartedInTheBackgroundEndsWithItsResponse(t *testing.T) {
	sample, setup := recorded(t)
	ws := t.TempDir()
	marker := filepath.Join(ws, "survived")
	command := "sleep 2; touch " + marker

	copyTo := func(from, to string, mode os.FileMode) {
		b, err := os.ReadFile(from)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(to), 0o755))
		require.NoError(t, os.WriteFile(to, b, mode))
	}
	copyTo(filepath.Join(setup, "hooks.json"), filepath.Join(ws, ".cursor", "hooks.json"), 0o644)
	copyTo(filepath.Join(setup, "hook.sh"), filepath.Join(ws, ".cursor", "hooks", "hook.sh"), 0o755)

	sub := filepath.Join(ws, "sub.sh")
	require.NoError(t, os.WriteFile(sub, []byte(`#!/bin/sh
if grep -q tool_use "$A10N_MOCK_SESSION_FILE"; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"DONE"}]}}'
  printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"result":"DONE"}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_bg","name":"Shell","input":{"command":"`+command+`","block_until_ms":0}}]}}'
`), 0o755))
	main := filepath.Join(ws, "main.sh")
	require.NoError(t, os.WriteFile(main, []byte(`#!/bin/sh
if grep -q CHECKED "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"result":"FINISHED"}'
  exit 0
fi
if grep -q '"name":"Task"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_check","name":"Shell","input":{"command":"pgrep -f \"sleep 2; [t]ouch `+marker+`\"; echo CHECKED"}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_task","name":"Task","input":{"description":"bg","prompt":"start it, answer DONE","subagent_type":"shell","run_in_background":false,"script":"`+sub+`"}}]}}'
`), 0o755))

	log := filepath.Join(ws, "payloads.jsonl")
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "--script", main, "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "HOOK_LOG=" + log}
	out, err := cmd.Output()
	require.NoError(t, err, "%s", out)

	var frames []map[string]any
	for _, l := range strings.Split(string(out), "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil && f != nil {
			frames = append(frames, f)
		}
	}
	got, want := jsonLines(t, log), jsonLines(t, filepath.Join(sample, "payloads.jsonl"))

	// the sub-agent's call: the same hooks, in the same order, as recorded, and
	// the receipt carries the shell and its process
	gotEvents, gotReceipt, subSession := backgroundOf(got, command)
	wantEvents, wantReceipt, _ := backgroundOf(want, "sh -c 'echo $$ > bgpid; exec sleep 47'")
	require.Equal(t, []string{"preToolUse", "beforeShellExecution", "postToolUse"}, wantEvents)
	require.Equal(t, wantEvents, gotEvents)
	var receipt struct {
		ShellID int `json:"shell_id"`
		Pid     int `json:"pid"`
	}
	require.NoError(t, json.Unmarshal([]byte(gotReceipt), &receipt))
	require.NotZero(t, receipt.ShellID)
	require.NotZero(t, receipt.Pid)
	require.Contains(t, wantReceipt, `"shell_id"`)
	require.Contains(t, wantReceipt, `"pid"`)

	// the sub-agent is a conversation of its own, and its calls are not on the
	// stream: the stream has the Task call and the run's own Shell-less result
	var parent string
	for _, p := range got {
		if p["hook_event_name"] == "sessionStart" {
			parent, _ = p["session_id"].(string)
		}
	}
	require.NotEmpty(t, parent)
	require.NotEqual(t, parent, subSession)
	var kinds []string
	for _, f := range frames {
		if tc, ok := f["tool_call"].(map[string]any); ok {
			for k := range tc {
				if strings.HasSuffix(k, "ToolCall") { // the call itself, not its envelope
					kinds = append(kinds, f["subtype"].(string)+"/"+k)
				}
			}
		}
	}
	require.Equal(t, []string{"started/taskToolCall", "completed/taskToolCall", "started/shellToolCall", "completed/shellToolCall"}, kinds)

	// the command is already gone when the main agent next runs a command, as
	// recorded (its ps finds none): not only at the end of the run
	var check string
	for _, p := range got {
		if p["hook_event_name"] == "afterShellExecution" {
			check, _ = p["output"].(string)
		}
	}
	require.Equal(t, "CHECKED\n", check)

	// the command is gone: its process is not running, and what it would have
	// done after the sub-agent answered never happens
	require.ErrorIs(t, syscall.Kill(receipt.Pid, 0), syscall.ESRCH, "the background command still runs")
	time.Sleep(2500 * time.Millisecond)
	_, err = os.Stat(marker)
	require.True(t, os.IsNotExist(err), "the command was left to finish")

	// and the stream says so, as recorded: an aborted task_notification of the
	// run's own session, titled with the command, for the shell of the receipt
	notes := notifications(frames)
	require.Len(t, notes, 1)
	require.Equal(t, "aborted", notes[0]["status"])
	order := streamOrder(frames)
	recStream := streamOrder(jsonLines(t, filepath.Join(sample, "stream.jsonl")))
	wantOrder := []string{"task started", "task done", "shell started", "shell done", "aborted", "result"}
	require.Equal(t, wantOrder, recStream, "the recording")
	require.Equal(t, wantOrder, order, "the notice follows the parent's next tool call and comes before the result, as recorded")
	require.Equal(t, command, notes[0]["title"])
	require.Equal(t, parent, notes[0]["session_id"])
	require.EqualValues(t, receipt.ShellID, mustAtoi(t, notes[0]["task_id"]))
	recNotes := notifications(jsonLines(t, filepath.Join(sample, "stream.jsonl")))
	require.Len(t, recNotes, 1)
	require.Equal(t, "aborted", recNotes[0]["status"])
	require.Equal(t, "sh -c 'echo $$ > bgpid; exec sleep 47'", recNotes[0]["title"])
}

// streamOrder is the tool calls, task notices and result of a stream, in order
// (the thinking and message frames left out).
func streamOrder(frames []map[string]any) []string {
	var order []string
	for _, f := range frames {
		if tc, ok := f["tool_call"].(map[string]any); ok {
			kind := "shell"
			if tc["taskToolCall"] != nil {
				kind = "task"
			}
			word := "started"
			if f["subtype"] == "completed" {
				word = "done"
			}
			order = append(order, kind+" "+word)
			continue
		}
		switch {
		case f["subtype"] == "task_notification":
			order = append(order, f["status"].(string))
		case f["type"] == "result":
			order = append(order, "result")
		}
	}
	return order
}

func mustAtoi(t *testing.T, v any) int {
	t.Helper()
	s, ok := v.(string)
	require.True(t, ok, "task_id %v", v)
	var n int
	require.NoError(t, json.Unmarshal([]byte(s), &n))
	return n
}
