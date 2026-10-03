package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/print-waits-for-background-agents: a background Task
// is launched and the turn ends at once; the print session stays open, the
// sub-agent's end is announced (system/task_notification), a further turn
// handles it, and only then does the result come.

// streamKinds are the frames of a run's stream that carry the sub-agent's
// story, in order: the Task call's two frames, the notification, the result.
func streamKinds(t *testing.T, stream string) (kinds []string, frames []map[string]any) {
	t.Helper()
	for _, f := range readJSONLText(t, stream) {
		typ, _ := f["type"].(string)
		sub, _ := f["subtype"].(string)
		tc, _ := f["tool_call"].(map[string]any)
		switch {
		case typ == "tool_call" && tc["taskToolCall"] != nil:
			kinds, frames = append(kinds, "task/"+sub), append(frames, f)
		case typ == "system" && sub == "task_notification", typ == "result":
			kinds, frames = append(kinds, typ+"/"+sub), append(frames, f)
		}
	}
	return kinds, frames
}

func readJSONLText(t *testing.T, text string) (out []map[string]any) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "s.jsonl")
	require.NoError(t, os.WriteFile(f, []byte(text), 0o644))
	return readJSONL(t, f)
}

// TestPrintSessionStaysOpenForABackgroundAgentAndItsEndStartsAFurtherTurn:
// recorded, a background Task is answered at once (isBackground), the agent's
// turn ends, and the session does not: the sub-agent's end is announced with a
// task_notification frame (its id, its prompt as the title, what it said as the
// detail), a further turn answers it, and the result is the last frame. The
// mock does the same, and the session-end hook fires only after the sub-agent
// has finished.
// sr:proves print-waits-for-background-agents/cursor
func TestPrintSessionStaysOpenForABackgroundAgentAndItsEndStartsAFurtherTurn(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	samples, err := filepath.Glob(filepath.Join(filepath.Dir(file), "..", "..", "snapshots", "runs", "print-waits-for-background-agents", "samples", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, samples)
	sort.Strings(samples)
	recorded, err := os.ReadFile(filepath.Join(samples[len(samples)-1], "stream.jsonl"))
	require.NoError(t, err)
	want, wantFrames := streamKinds(t, string(recorded))
	require.Equal(t, []string{"task/started", "task/completed", "system/task_notification", "result/success"}, want)
	wantResult := wantFrames[1]["tool_call"].(map[string]any)["taskToolCall"].(map[string]any)["result"].(map[string]any)["success"].(map[string]any)
	require.Equal(t, true, wantResult["isBackground"])
	require.Equal(t, wantResult["agentId"], wantFrames[2]["task_id"], "the notification names the sub-agent the call answered with")

	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	scratch := t.TempDir()
	done := filepath.Join(scratch, "subagent-done")
	log := filepath.Join(scratch, "log")
	put := func(path, body string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o755))
	}
	put(filepath.Join(ws, ".cursor", "hooks.json"), `{"version":1,"hooks":{"sessionEnd":[{"command":".cursor/hooks/end.sh"}]}}`)
	put(filepath.Join(ws, ".cursor", "hooks", "end.sh"), "#!/bin/sh\ncat >/dev/null\nif [ -f "+done+" ]; then echo ended-after-subagent >>"+log+"; else echo ended-too-early >>"+log+"; fi\n")
	sub := filepath.Join(scratch, "sub.sh")
	put(sub, "#!/bin/sh\nsleep 1\ntouch "+done+"\n"+
		`echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"SUBREPLY"}]}}'`+"\n"+
		`echo '{"type":"result","subtype":"success","result":"SUBREPLY"}'`+"\n")
	script := filepath.Join(scratch, "main.sh")
	put(script, `#!/bin/sh
say() { echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"'"$1"'"}]}}'; echo '{"type":"result","subtype":"success","result":"'"$1"'"}'; }
case "$A10N_MOCK_PROMPT" in
Perform*) say FOLLOWED-UP ;;
*) if grep -q '"name":"Task"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then say LAUNCHED; else
echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Task","input":{"description":"sleeper","prompt":"sleep then reply","subagent_type":"shell","run_in_background":true,"script":"`+sub+`"}}]}}'; fi ;;
esac
`)
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "A10N_MOCK_SCRIPT=" + script}
	began := time.Now()
	out, err := cmd.Output()
	require.NoError(t, err, "%s", out)
	require.GreaterOrEqual(t, time.Since(began), time.Second, "the run outlived the turn that launched the sub-agent")

	got, gotFrames := streamKinds(t, string(out))
	require.Equal(t, want, got)
	result := gotFrames[1]["tool_call"].(map[string]any)["taskToolCall"].(map[string]any)["result"].(map[string]any)["success"].(map[string]any)
	require.Equal(t, true, result["isBackground"])
	note := gotFrames[2]
	require.Equal(t, result["agentId"], note["task_id"])
	require.Equal(t, "success", note["status"])
	require.Equal(t, "sleep then reply", note["title"])
	require.Equal(t, "SUBREPLY", note["detail"])
	require.Equal(t, "LAUNCHEDFOLLOWED-UP", gotFrames[3]["result"], "the further turn's words are in the result")
	b, err := os.ReadFile(log)
	require.NoError(t, err)
	require.Equal(t, "ended-after-subagent", strings.TrimSpace(string(b)))
}
