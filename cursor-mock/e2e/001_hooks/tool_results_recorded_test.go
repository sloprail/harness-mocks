package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// shellBodies are the result objects (failure or success) of the completed
// shell tool-call frames of a stream, by the command they ran, without the
// timings (executionTime, localExecutionTimeMs), which differ between runs.
func shellBodies(frames []map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, f := range frames {
		if f["type"] != "tool_call" || f["subtype"] != "completed" {
			continue
		}
		call, ok := f["tool_call"].(map[string]any)["shellToolCall"].(map[string]any)
		if !ok {
			continue
		}
		res, _ := call["result"].(map[string]any)
		for kind, v := range res {
			body, ok := v.(map[string]any)
			if !ok {
				continue
			}
			named := map[string]any{"kind": kind}
			for k, e := range body {
				if k != "executionTime" && k != "localExecutionTimeMs" {
					named[k] = e
				}
			}
			out[body["command"].(string)] = named
		}
	}
	return out
}

func streamOf(t *testing.T, run string) []map[string]any {
	t.Helper()
	return readJSONL(t, filepath.Join(newestSample(t, run), "stream.jsonl"))
}

func printedFrames(stdout string) (out []map[string]any) {
	for _, l := range strings.Split(stdout, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil {
			out = append(out, f)
		}
	}
	return out
}

// hooksOf are a run's hook payloads of an event, for the call the command (a
// shell line) or the path (a Read or Write's file) names.
func hooksOf(o observed, event, subject string) (out []map[string]any) {
	for _, h := range o.hooks {
		if h["hook_event_name"] != event {
			continue
		}
		in, _ := h["tool_input"].(map[string]any)
		if in["command"] == subject || in["file_path"] == subject || h["command"] == subject {
			out = append(out, h)
		}
	}
	return out
}

// TestAStderrOnlyFailureIsAFailureWithItsOutputExitCodeAndHookMessage:
// recorded (runs/tool-failure: `false`, which prints nothing, and `sh -c 'echo
// OOPS >&2; exit 3'`, which prints only on stderr), a command exiting non-zero
// is a failure result carrying its exit code and both streams (stderr kept
// apart, and in the interleaved output), the afterShellExecution hook reports
// what it printed (stderr included), and the failure hook's error_message is
// that output, "OOPS", or "Command failed with exit code 1" for a command that
// printed nothing, with failure_type "error". The mock's frames carry the same
// objects, field for field, apart from the timings.
// sr:proves bash-tool-result/cursor
func TestAStderrOnlyFailureIsAFailureWithItsOutputExitCodeAndHookMessage(t *testing.T) {
	got, want := replay(t, "tool-failure")
	conforms(t, got, want)

	recorded := shellBodies(streamOf(t, "tool-failure"))
	mock := shellBodies(printedFrames(got.stdout))
	const oops = "sh -c 'echo OOPS >&2; exit 3'"
	require.Equal(t, "failure", recorded[oops]["kind"])
	require.EqualValues(t, 3, recorded[oops]["exitCode"])
	require.Equal(t, "", recorded[oops]["stdout"], "recorded: nothing on stdout")
	require.Equal(t, "OOPS\n", recorded[oops]["stderr"])
	require.Equal(t, "OOPS\n", recorded[oops]["interleavedOutput"])
	require.Equal(t, false, recorded[oops]["aborted"])
	require.Equal(t, recorded, mock, "every command's result object, as recorded")

	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		for cmd, why := range map[string]struct{ message, output string }{
			"false": {"Command failed with exit code 1", ""},
			oops:    {"OOPS", "OOPS\n"},
		} {
			failures := hooksOf(o, "postToolUseFailure", cmd)
			require.Len(t, failures, 1, name+": "+cmd)
			require.Equal(t, why.message, failures[0]["error_message"], name+": "+cmd)
			require.Equal(t, "error", failures[0]["failure_type"], name+": "+cmd)
			require.Equal(t, false, failures[0]["is_interrupt"], name+": "+cmd)
			after := hooksOf(o, "afterShellExecution", cmd)
			require.Len(t, after, 1, name+": "+cmd)
			require.Equal(t, why.output, after[0]["output"], name+": "+cmd)
		}
	}
}

// TestAFailedOrRefusedCallFiresPostToolUseFailureAndNeverPostToolUse: recorded
// (runs/tool-failure, runs/pretool-refusal), postToolUse fires only for a call
// that ran and succeeded; a command that exited non-zero and a read of a file
// that is not there fire postToolUseFailure (failure_type error, the tool's
// error as error_message, is_interrupt false) and no postToolUse, and a call a
// hook refused fires postToolUseFailure with permission_denied and no
// postToolUse. The calls that succeeded carry their structured result as
// tool_output, exactly as recorded.
// sr:proves posttooluse-payload/cursor
func TestAFailedOrRefusedCallFiresPostToolUseFailureAndNeverPostToolUse(t *testing.T) {
	got, want := replay(t, "tool-failure")
	conforms(t, got, want)
	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		for _, subject := range []string{"false", "sh -c 'echo OOPS >&2; exit 3'", "<RUN>/missing-file.txt"} {
			require.Empty(t, hooksOf(o, "postToolUse", subject), name+": "+subject+" failed, so no postToolUse")
			failures := hooksOf(o, "postToolUseFailure", subject)
			require.Len(t, failures, 1, name+": "+subject)
			require.Equal(t, "error", failures[0]["failure_type"], name+": "+subject)
			require.Equal(t, false, failures[0]["is_interrupt"], name+": "+subject)
		}
		require.Equal(t, "File not found: <RUN>/missing-file.txt", hooksOf(o, "postToolUseFailure", "<RUN>/missing-file.txt")[0]["error_message"], name)
		var posts []string
		for _, p := range postToolUses(o) {
			posts = append(posts, p["tool"].(string)+" "+p["output"].(string))
		}
		require.Equal(t, []string{
			`Write {"file_path":"<RUN>/note.txt","success":true}`,
			`Shell {"output":"FINE\n","exitCode":0}`,
		}, posts, name+": only the write and the command that succeeded")
	}

	refused, rwant := replay(t, "pretool-refusal")
	conforms(t, refused, rwant)
	for name, o := range map[string]observed{"recorded": rwant, "mock": refused} {
		for _, cmd := range []string{"echo DENYME", "echo EXIT2PRE", "echo EXIT2SHELL", "echo DENYSHELL", "echo DENYWINS"} {
			require.Empty(t, hooksOf(o, "postToolUse", cmd), name+": "+cmd+" was refused, so no postToolUse")
			failures := hooksOf(o, "postToolUseFailure", cmd)
			require.Len(t, failures, 1, name+": "+cmd)
			require.Equal(t, "permission_denied", failures[0]["failure_type"], name+": "+cmd)
			require.Equal(t, false, failures[0]["is_interrupt"], name+": "+cmd)
		}
		finePosts := hooksOf(o, "postToolUse", "echo FINE")
		require.Len(t, finePosts, 1, name)
		require.JSONEq(t, `{"output":"FINE\n","exitCode":0}`, finePosts[0]["tool_output"].(string), name)
	}
}
