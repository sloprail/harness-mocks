package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// custom is a run the tests set up themselves, where no recording's setup
// fits: a hooks.json, the hook scripts it names, and the commands the script
// asks the agent to run.
type custom struct {
	ws, home, log string
	stdout        string
}

func runCustom(t *testing.T, hooksJSON string, scripts map[string]string, commands ...string) custom {
	t.Helper()
	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	scratch, home := t.TempDir(), t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".cursor", "hooks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".cursor", "hooks.json"), []byte(hooksJSON), 0o644))
	for name, body := range scripts {
		require.NoError(t, os.WriteFile(filepath.Join(ws, ".cursor", "hooks", name), []byte(body), 0o755))
	}
	for i, c := range commands {
		call := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_` + strconv.Itoa(i) + `","name":"Bash","input":{"command":` + jsonString(c) + `}}]}}`
		require.NoError(t, os.WriteFile(filepath.Join(scratch, strconv.Itoa(i)+".json"), []byte(call+"\n"), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(scratch, "end.json"), []byte(`{"type":"result","subtype":"success","result":"DONE"}`+"\n"), 0o644))
	script := filepath.Join(scratch, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nn=$(grep -c '\"type\":\"tool_use\"' \"$A10N_MOCK_SESSION_FILE\" 2>/dev/null)\nn=${n:-0}\nf=\""+scratch+"/$n.json\"\n[ -f \"$f\" ] || f=\""+scratch+"/end.json\"\ncat \"$f\"\n"), 0o755))
	logPath := filepath.Join(scratch, "log.txt")
	cmd := exec.Command(binary, "-p", "--output-format", "stream-json", "--script", script, "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "HOOK_LOG=" + logPath}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return custom{ws: ws, home: home, log: logPath, stdout: string(out)}
}

func (c custom) logged(t *testing.T) string {
	t.Helper()
	b, _ := os.ReadFile(c.log)
	return string(b)
}

// transcript is the conversation's transcript file, found under the home the
// way Cursor lays it out: .cursor/projects/<workspace>/agent-transcripts/<id>/<id>.jsonl.
func (c custom) transcript(t *testing.T) (path, session string) {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(c.home, ".cursor", "projects", "*", "agent-transcripts", "*", "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, m, 1)
	return m[0], filepath.Base(filepath.Dir(m[0]))
}

// TestAHookCommandRunsThroughTheShellInTheProjectRootWithThePayloadOnStdin:
// recorded, a project hook given as a shell line (with arguments, a relative
// path and shell syntax) runs from the project root and reads its event's
// payload, as JSON, on stdin.
// sr:proves hook-command-handler/cursor
func TestAHookCommandRunsThroughTheShellInTheProjectRootWithThePayloadOnStdin(t *testing.T) {
	got, want := replay(t, "hook-matchers")
	conforms(t, got, want)

	c := runCustom(t, `{"version":1,"hooks":{"afterShellExecution":[{"command":"cat > \"$HOOK_LOG.in\"; pwd -P > \"$HOOK_LOG.pwd\"; .cursor/hooks/args.sh one"}]}}`,
		map[string]string{"args.sh": "#!/bin/sh\necho \"$1\" > \"$HOOK_LOG.arg\"\n"}, "echo hi")
	pwd, _ := os.ReadFile(c.log + ".pwd")
	require.Equal(t, c.ws, strings.TrimSpace(string(pwd)), "a project hook runs from the project root")
	in, _ := os.ReadFile(c.log + ".in")
	require.Contains(t, string(in), `"hook_event_name":"afterShellExecution"`)
	arg, _ := os.ReadFile(c.log + ".arg")
	require.Equal(t, "one", strings.TrimSpace(string(arg)), "a shell line with a relative path and an argument")
	require.Contains(t, string(in), `"command":"echo hi"`)
}

// TestTheTranscriptFileIsKeyedByTheProjectAndTheSessionAndAbsentAtStart:
// recorded, the transcript is .cursor/projects/<workspace path with every
// non-alphanumeric character as "-">/agent-transcripts/<session>/<session>.jsonl;
// the start hook's payload has no transcript path, and later ones have it.
// sr:proves session-transcript-file/cursor
func TestTheTranscriptFileIsKeyedByTheProjectAndTheSessionAndAbsentAtStart(t *testing.T) {
	c := runCustom(t, `{"version":1,"hooks":{"sessionStart":[{"command":"cat > \"$HOOK_LOG.start\"; ls -R \"$HOME/.cursor/projects\" > \"$HOOK_LOG.tree\" 2>&1"}],"beforeShellExecution":[{"command":"cat >> \"$HOOK_LOG\""}]}}`, nil, "echo hi")
	path, session := c.transcript(t)
	project := strings.NewReplacer("/", "-", ".", "-", "_", "-").Replace(strings.TrimPrefix(c.ws, "/"))
	require.Equal(t, filepath.Join(c.home, ".cursor", "projects", project, "agent-transcripts", session, session+".jsonl"), path)

	start, _ := os.ReadFile(c.log + ".start")
	require.Contains(t, string(start), `"transcript_path":null`)
	tree, _ := os.ReadFile(c.log + ".tree")
	require.NotContains(t, string(tree), session+".jsonl", "the file does not exist yet when the start hook runs")
	require.Contains(t, c.logged(t), `"transcript_path":"`+path+`"`)
}

// TestWhateverTheSessionEndHookPrintsIsNotInTheTranscript: recorded, the
// sessionEnd hook fires last, once, with the reason "completed" (a
// non-interactive run has no other), and nothing it prints reaches the
// transcript.
// sr:proves session-end-hook/cursor
func TestWhateverTheSessionEndHookPrintsIsNotInTheTranscript(t *testing.T) {
	got, want := replay(t, "pretool-refusal")
	conforms(t, got, want)
	last := got.raw[len(got.raw)-1]
	require.Equal(t, "sessionEnd", last["hook_event_name"])
	require.Equal(t, "completed", last["reason"])
	require.Equal(t, "completed", last["final_status"])

	c := runCustom(t, `{"version":1,"hooks":{"sessionEnd":[{"command":"cat >/dev/null; echo SESSION-END-OUTPUT; echo SESSION-END-OUTPUT >&2"}]}}`, nil, "echo hi")
	path, _ := c.transcript(t)
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(b), "SESSION-END-OUTPUT")
}

// TestEveryHookPayloadNamesTheSessionTheTranscriptAndTheWorkspace: recorded,
// each payload carries the conversation and session id (the same one), the
// workspace roots and the transcript path.
// sr:proves hook-common-payload/cursor
func TestEveryHookPayloadNamesTheSessionTheTranscriptAndTheWorkspace(t *testing.T) {
	got, want := replay(t, "tool-failure")
	conforms(t, got, want)
	sid, _ := got.raw[0]["session_id"].(string)
	require.NotEmpty(t, sid)
	for i, h := range got.raw {
		require.Equal(t, sid, h["session_id"], h["hook_event_name"])
		require.Equal(t, sid, h["conversation_id"])
		require.Equal(t, []any{got.ws}, h["workspace_roots"])
		if i == 0 {
			require.Nil(t, h["transcript_path"], "no transcript yet at sessionStart")
			continue
		}
		require.Contains(t, h["transcript_path"], sid+".jsonl", h["hook_event_name"])
	}
}

// TestAfterAToolSucceedsTheHookGetsItsInputItsOutputAndHowLongItTook:
// recorded, postToolUse carries the call's tool_input, its tool_output (the
// JSON of the structured result) and a duration in milliseconds.
// sr:proves posttooluse-payload/cursor
func TestAfterAToolSucceedsTheHookGetsItsInputItsOutputAndHowLongItTook(t *testing.T) {
	got, want := replay(t, "file-tools")
	conforms(t, got, want)
	n := 0
	for _, h := range got.raw {
		if h["hook_event_name"] != "postToolUse" {
			continue
		}
		n++
		require.NotEmpty(t, h["tool_input"])
		require.NotEmpty(t, h["tool_output"])
		_, ok := h["duration"].(float64)
		require.True(t, ok, "duration in ms: %v", h)
	}
	require.Equal(t, 4, n, "the write, the read, and the write that follows with the read it makes first")
}
