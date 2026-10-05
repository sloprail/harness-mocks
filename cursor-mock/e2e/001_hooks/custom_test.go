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
	return runCustomAt(t, false, hooksJSON, scripts, commands...)
}

// runCustomAt is runCustom, optionally started from a symlink to the
// workspace (c.ws is still the workspace's real path).
func runCustomAt(t *testing.T, symlinked bool, hooksJSON string, scripts map[string]string, commands ...string) custom {
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
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nn=$(grep -c '\"type\":\"tool_use\"' \"$A10N_MOCK_SESSION_FILE\" 2>/dev/null)\nn=${n:-0}\nprintf '%s\\n' \"$A10N_MOCK_ADDITIONAL_CONTEXT\" | tr '\\n' ' ' >>\"$HOOK_LOG.ctx\"; echo >>\"$HOOK_LOG.ctx\"\nf=\""+scratch+"/$n.json\"\n[ -f \"$f\" ] || f=\""+scratch+"/end.json\"\ncat \"$f\"\n"), 0o755))
	logPath := filepath.Join(scratch, "log.txt")
	cmd := exec.Command(binary, "-p", "--force", "--output-format", "stream-json", "--script", script, "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "HOOK_LOG=" + logPath}
	if symlinked {
		link := filepath.Join(t.TempDir(), "link")
		require.NoError(t, os.Symlink(ws, link))
		cmd.Dir, cmd.Env = link, append(cmd.Env, "PWD="+link)
	}
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
// the start hook's payload and the first preToolUse's have no transcript path,
// and the payloads from the first command's afterShellExecution on have it; in
// between, the first beforeShellExecution's varies from one capture to the
// next (named in runs/tool-failure, runs/symlinked-cwd, null in
// runs/shell-exit-status, runs/hook-json-nonzero), and the mock names it there.
// sr:proves session-transcript-file/cursor
func TestTheTranscriptFileIsKeyedByTheProjectAndTheSessionAndAbsentAtStart(t *testing.T) {
	c := runCustom(t, `{"version":1,"hooks":{"sessionStart":[{"command":"cat > \"$HOOK_LOG.start\"; ls -R \"$HOME/.cursor/projects\" > \"$HOOK_LOG.tree\" 2>&1"}],"preToolUse":[{"command":"cat > \"$HOOK_LOG.pre\""}],"beforeShellExecution":[{"command":"cat > \"$HOOK_LOG.before\""}],"afterShellExecution":[{"command":"cat >> \"$HOOK_LOG\""}]}}`, nil, "echo hi")
	path, session := c.transcript(t)
	project := strings.NewReplacer("/", "-", ".", "-", "_", "-").Replace(strings.TrimPrefix(c.ws, "/"))
	require.Equal(t, filepath.Join(c.home, ".cursor", "projects", project, "agent-transcripts", session, session+".jsonl"), path)

	start, _ := os.ReadFile(c.log + ".start")
	require.Contains(t, string(start), `"transcript_path":null`)
	tree, _ := os.ReadFile(c.log + ".tree")
	require.NotContains(t, string(tree), session+".jsonl", "the file does not exist yet when the start hook runs")
	pre, _ := os.ReadFile(c.log + ".pre")
	require.Contains(t, string(pre), `"transcript_path":null`, "the first preToolUse names no transcript")
	before, _ := os.ReadFile(c.log + ".before")
	require.Contains(t, string(before), `"transcript_path":"`+path+`"`, "the mock names it at the first beforeShellExecution")
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
// each payload carries the conversation and session id (the same one) and the
// workspace roots; its transcript path is null at sessionStart and in the
// first preToolUse, and the transcript file's from then on.
// sr:proves hook-common-payload/cursor
func TestEveryHookPayloadNamesTheSessionTheTranscriptAndTheWorkspace(t *testing.T) {
	got, want := replay(t, "tool-failure")
	conforms(t, got, want)
	sid, _ := got.raw[0]["session_id"].(string)
	require.NotEmpty(t, sid)
	firstPre := true
	for _, h := range got.raw {
		require.Equal(t, sid, h["session_id"], h["hook_event_name"])
		require.Equal(t, sid, h["conversation_id"])
		require.Equal(t, []any{got.ws}, h["workspace_roots"])
		if h["hook_event_name"] == "sessionStart" || (h["hook_event_name"] == "preToolUse" && firstPre) {
			require.Nil(t, h["transcript_path"], "no transcript path yet: %v", h["hook_event_name"])
			firstPre = firstPre && h["hook_event_name"] != "preToolUse"
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
		_, ok := h["duration"].(float64)
		require.True(t, ok, "duration in ms: %v", h)
	}
	require.Equal(t, 4, n, "the write, the read, and the write that follows with the read it makes first")
	// each call's tool_input and tool_output are the recorded ones, value for value
	require.Equal(t, []map[string]any{
		{"tool": "Write", "input": map[string]any{"file_path": "<RUN>/note.txt", "content": "hi\n"}, "output": `{"file_path":"<RUN>/note.txt","success":true}`},
		{"tool": "Read", "input": map[string]any{"file_path": "<RUN>/note.txt"}, "output": `{"file_path":"<RUN>/note.txt","content_length":3}`},
		{"tool": "Read", "input": map[string]any{"file_path": "<RUN>/note.txt"}, "output": `{"file_path":"<RUN>/note.txt","content_length":3}`},
		{"tool": "Write", "input": map[string]any{"file_path": "<RUN>/note.txt", "content": "bye\n"}, "output": `{"file_path":"<RUN>/note.txt","success":true}`},
	}, postToolUses(want), "recorded")
	require.Equal(t, postToolUses(want), postToolUses(got), "mock")
}

// TestADenyFromOneHookRefusesTheCallWhateverAnotherHookAsks: docs, any deny
// wins over ask: with one preToolUse hook asking and another denying the same
// call, in either order, the call is refused and does not run.
// sr:proves pretooluse-refusal/cursor
func TestADenyFromOneHookRefusesTheCallWhateverAnotherHookAsks(t *testing.T) {
	scripts := map[string]string{
		"ask.sh":  "#!/bin/sh\ncat >/dev/null\necho '{\"permission\":\"ask\",\"user_message\":\"ASK-MSG\"}'\n",
		"deny.sh": "#!/bin/sh\ncat >/dev/null\necho '{\"permission\":\"deny\",\"user_message\":\"DENY-MSG\"}'\n",
	}
	for name, order := range map[string]string{
		"ask first":  `[{"command":".cursor/hooks/ask.sh"},{"command":".cursor/hooks/deny.sh"}]`,
		"deny first": `[{"command":".cursor/hooks/deny.sh"},{"command":".cursor/hooks/ask.sh"}]`,
	} {
		c := runCustom(t, `{"version":1,"hooks":{"preToolUse":`+order+`,"afterShellExecution":[{"command":"cat >> \"$HOOK_LOG\""}]}}`, scripts, "echo REFUSED-OR-NOT")
		require.Contains(t, c.stdout, `"rejected"`, name)
		require.Contains(t, c.stdout, "DENY-MSG", name)
		require.NotContains(t, c.stdout, "ASK-MSG", name)
		require.NotContains(t, c.logged(t), "REFUSED-OR-NOT", name+": the command must not have run")
	}
}

// TestTheUserRecordOpensWithAnEmptyTimestampElementThenTheQuery: recorded
// (runs/tool-failure, runs/symlinked-cwd), the transcript's first record is the
// user's, a text block that starts with an empty <timestamp/> element on its
// own line and then holds the prompt in a <user_query> element; the mock
// writes the same.
func TestTheUserRecordOpensWithAnEmptyTimestampElementThenTheQuery(t *testing.T) {
	first := func(recs []map[string]any) string {
		require.NotEmpty(t, recs)
		require.Equal(t, "user", recs[0]["role"])
		blocks := recs[0]["message"].(map[string]any)["content"].([]any)
		require.Len(t, blocks, 1)
		return blocks[0].(map[string]any)["text"].(string)
	}
	for _, run := range []string{"tool-failure", "symlinked-cwd"} {
		m, err := filepath.Glob(filepath.Join(newestSample(t, run), "transcript", "*", "*.jsonl"))
		require.NoError(t, err)
		require.Len(t, m, 1, run)
		require.True(t, strings.HasPrefix(first(readJSONL(t, m[0])), "<timestamp/>\n<user_query>\n"), run)
	}

	c := runCustom(t, `{"version":1,"hooks":{}}`, nil, "echo hi")
	path, _ := c.transcript(t)
	got := first(readJSONL(t, path))
	require.True(t, strings.HasPrefix(got, "<timestamp/>\n<user_query>\n"), got)
	require.True(t, strings.HasSuffix(got, "\n</user_query>"), got)
}

// TestADenyBeatsAskAndRefusalMessagesAreJoinedOnBeforeShellExecutionAndBeforeReadFile:
// the docs (#configuration) say all matching hooks run, any deny wins over ask,
// and the hooks' messages are concatenated; recorded only for preToolUse
// (runs/pretool-refusal-combined), so this drives the mock on the other two
// events that refuse: with one beforeShellExecution hook asking and another
// denying, in either order, the command is refused with the deny's message and
// does not run; two hooks that both deny a command, or a read, have their
// messages joined in the order the hooks are configured in.
// sr:proves hooks-all-matching-run/cursor
func TestADenyBeatsAskAndRefusalMessagesAreJoinedOnBeforeShellExecutionAndBeforeReadFile(t *testing.T) {
	scripts := map[string]string{
		"ask.sh":   "#!/bin/sh\ncat >/dev/null\necho '{\"permission\":\"ask\",\"user_message\":\"ASK-MSG\"}'\n",
		"deny1.sh": "#!/bin/sh\ncat >/dev/null\necho '{\"permission\":\"deny\",\"user_message\":\"DENY-ONE\"}'\n",
		"deny2.sh": "#!/bin/sh\ncat >/dev/null\necho '{\"permission\":\"deny\",\"user_message\":\"DENY-TWO\"}'\n",
	}
	for name, order := range map[string]string{
		"ask first":  `[{"command":".cursor/hooks/ask.sh"},{"command":".cursor/hooks/deny1.sh"}]`,
		"deny first": `[{"command":".cursor/hooks/deny1.sh"},{"command":".cursor/hooks/ask.sh"}]`,
	} {
		c := runCustom(t, `{"version":1,"hooks":{"beforeShellExecution":`+order+`,"afterShellExecution":[{"command":"cat >> \"$HOOK_LOG\""}]}}`, scripts, "echo REFUSED-OR-NOT")
		require.Contains(t, c.stdout, "DENY-ONE", name)
		require.NotContains(t, c.stdout, "ASK-MSG", name)
		require.NotContains(t, c.logged(t), "REFUSED-OR-NOT", name+": the command must not have run")
	}

	c := runCustom(t, `{"version":1,"hooks":{"beforeShellExecution":[{"command":".cursor/hooks/deny1.sh"},{"command":".cursor/hooks/deny2.sh"}]}}`, scripts, "echo BOTH")
	require.Contains(t, c.stdout, "DENY-ONE\\n\\n---\\n\\nDENY-TWO", "the command's refusal joins both messages in configured order")

	r := runTools(t, `{"version":1,"hooks":{"beforeReadFile":[{"command":".cursor/hooks/deny1.sh"},{"command":".cursor/hooks/deny2.sh"}]}}`, scripts,
		map[string]string{"note.txt": "hi\n"}, map[string]any{"name": "Read", "input": map[string]any{"file_path": "note.txt"}})
	var joined string
	for _, f := range r.frames {
		if tc, _ := f["tool_call"].(map[string]any); tc != nil {
			joined += jsonString(tc)
		}
	}
	require.Contains(t, joined, "DENY-ONE\\n\\n---\\n\\nDENY-TWO", "the read's refusal joins both messages in configured order")
}
