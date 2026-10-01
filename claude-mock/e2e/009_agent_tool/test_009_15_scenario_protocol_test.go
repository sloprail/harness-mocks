package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bashToolUse is a stream-json assistant line calling Bash with command.
func bashToolUse(id, command string) string {
	return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"` + id + `","name":"Bash","input":{"command":"` + command + `"}}]}}`
}

// The script reads the session so far through A10N_MOCK_SESSION_FILE: on its
// second turn the file holds its own first-turn tool_use and that tool's
// result, and it is the session's transcript file.
// sr:proves session-file-env
func TestT009_15_ScriptReadsTheSessionSoFar(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	seen, path := filepath.Join(dir, "seen"), filepath.Join(dir, "path")
	script := writeScript(t, dir, "s.sh", `#!/bin/sh
if grep -q '"tool_use_id":"t1"' "$A10N_MOCK_SESSION_FILE"; then
  cp "$A10N_MOCK_SESSION_FILE" "`+seen+`"
  printf %s "$A10N_MOCK_SESSION_FILE" > "`+path+`"
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '`+bashToolUse("t1", "echo FIRST-TURN-OUT")+`'
`)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-sf", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	got := readOrEmpty(seen)
	assert.Contains(t, got, `"id":"t1"`, "the first turn's tool_use")
	assert.Contains(t, got, "FIRST-TURN-OUT", "and its result")
	files := transcriptFiles(cfg)
	require.NotEmpty(t, files)
	assert.Contains(t, files, readOrEmpty(path), "the file is the session's transcript")
}

// The script runs once per turn: a tool_use line ends the turn, the mock runs
// the tool and runs the script again; a result line ends the run, and the
// script does not run after it.
// sr:proves turn-loop
func TestT009_15_OneScriptRunPerTurn(t *testing.T) {
	dir := t.TempDir()
	runs, ran := filepath.Join(dir, "runs"), filepath.Join(dir, "ran")
	script := writeScript(t, dir, "s.sh", `#!/bin/sh
echo run >> "`+runs+`"
if [ -f "`+ran+`" ]; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '`+bashToolUse("t1", "touch "+ran)+`'
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"AFTER-TOOL-USE"}]}}'
`)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-tl", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.True(t, fileExists(ran), "the tool ran between the turns")
	assert.Equal(t, 2, strings.Count(readOrEmpty(runs), "run"), "one run for the tool_use turn, one for the result")
	assert.NotContains(t, out, "AFTER-TOOL-USE", "a tool_use line ends the turn: what follows it is not read")
}

// A script that emits the same tool_use turn after turn makes the run abort
// with an error instead of looping forever.
// sr:proves loop-guard
func TestT009_15_SameToolUseRepeatedAborts(t *testing.T) {
	dir := t.TempDir()
	runs := filepath.Join(dir, "runs")
	script := writeScript(t, dir, "s.sh", `#!/bin/sh
echo run >> "`+runs+`"
printf '%s\n' '`+bashToolUse("t1", "true")+`'
`)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-lg", "--project-dir", dir, "-p", "go")
	assert.NotEqual(t, 0, code, "output:\n%s", out)
	assert.Contains(t, out, "scenario looped")
	n := strings.Count(readOrEmpty(runs), "run")
	assert.Equal(t, 5, n, "the 5th identical tool_use in a row stops the run")
}

// A different tool_use resets the count: 4 identical calls, then another,
// then the first again, then a result, run to the end.
// sr:proves loop-guard
func TestT009_15_ChangedToolUseResetsTheGuard(t *testing.T) {
	dir := t.TempDir()
	runs := filepath.Join(dir, "runs")
	script := writeScript(t, dir, "s.sh", `#!/bin/sh
echo run >> "`+runs+`"
n=$(wc -l < "`+runs+`" | tr -d ' ')
case "$n" in
  5) printf '%s\n' '`+bashToolUse("t2", "echo other")+`' ;;
  10) printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}' ;;
  *) printf '%s\n' '`+bashToolUse("t1", "true")+`' ;;
esac
`)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-lr", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "4 in a row, a different call, then 4 again never reach 5; output:\n%s", out)
	assert.NotContains(t, out, "scenario looped")
	assert.Equal(t, 10, strings.Count(readOrEmpty(runs), "run"))
}

// A sub-agent runs its own script: the Agent call's script field, else the one
// A10N_MOCK_SUBAGENT_SCRIPT names.
// sr:proves subagent-script
func TestT009_15_SubagentScriptFromEnv(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "sub-ran")
	sub := writeScript(t, dir, "sub.sh", `#!/bin/sh
: > "`+marker+`"
printf '%s\n' '{"type":"result","subtype":"success","result":"SUB","is_error":false}'
`)
	script := writeScript(t, dir, "s.sh", `#!/bin/sh
if grep -q 'agentId:' "$A10N_MOCK_SESSION_FILE"; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"a1","name":"Agent","input":{"description":"d","prompt":"p","subagent_type":"general-purpose"}}]}}'
`)
	out, code := runInDir(t, dir, []string{"A10N_MOCK_SUBAGENT_SCRIPT=" + sub}, "--script", script, "--session-id", "s-ss", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.True(t, fileExists(marker), "the sub-agent ran the env's script")
}

// --system-prompt reaches the script in A10N_MOCK_SYSTEM_PROMPT.
// sr:proves system-prompt-env
func TestT009_15_SystemPromptReachesTheScript(t *testing.T) {
	dir := t.TempDir()
	got := filepath.Join(dir, "sys")
	script := writeScript(t, dir, "s.sh", `#!/bin/sh
printf %s "$A10N_MOCK_SYSTEM_PROMPT" > "`+got+`"
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-sp", "--project-dir", dir, "--system-prompt", "You are SYS-PROMPT, with  two spaces", "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.Equal(t, "You are SYS-PROMPT, with  two spaces", readOrEmpty(got))
}

// Print mode forwards the script's raw stdout; with A10N_MOCK_PRINT_STREAM=1 it
// runs through the turn loop instead, so a tool_use runs its tool.
// sr:proves print-stream
func TestT009_15_PrintStreamRunsTheTurnLoop(t *testing.T) {
	for _, stream := range []bool{false, true} {
		dir := t.TempDir()
		ran := filepath.Join(dir, "ran")
		script := writeScript(t, dir, "s.sh", `#!/bin/sh
if [ -f "`+ran+`" ]; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '`+bashToolUse("t1", "touch "+ran)+`'
`)
		var env []string
		if stream {
			env = []string{"A10N_MOCK_PRINT_STREAM=1"}
		}
		out, code := runInDir(t, dir, env, "--print", "--script", script, "--session-id", "s-ps", "--project-dir", dir, "-p", "go")
		require.Equal(t, 0, code, "output:\n%s", out)
		assert.Equal(t, stream, fileExists(ran), "the tool runs only through the turn loop (stream=%v)", stream)
		if !stream {
			assert.Contains(t, out, `"name":"Bash"`, "raw print mode forwards the line as text")
		}
	}
}

// A10N_MOCK_NO_RESUME=1 makes every --resume behave as one naming an unknown
// session, an existing one included: the same resume that succeeds without it
// fails with "No conversation found" and runs no script.
// sr:proves no-resume
func TestT009_15_NoResumeHidesAnExistingSession(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	runs := filepath.Join(dir, "runs")
	script := writeScript(t, dir, "s.sh", `#!/bin/sh
echo run >> "`+runs+`"
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`)
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-nr", "--project-dir", dir, "--config-dir", cfg, "-p", "first")
	require.Equal(t, 0, code, "the session exists; output:\n%s", out)

	out, code = runInDir(t, dir, []string{"A10N_MOCK_NO_RESUME=1"}, "--resume", "s-nr", "--script", script, "--project-dir", dir, "--config-dir", cfg, "--output-format", "stream-json", "-p", "again")
	assert.Equal(t, 1, code, "output:\n%s", out)
	assert.Contains(t, out, "No conversation found with session ID: s-nr")
	assert.Contains(t, out, `"is_error":true`)
	assert.Equal(t, 1, strings.Count(readOrEmpty(runs), "run"), "the resume ran no script")

	out, code = runInDir(t, dir, nil, "--resume", "s-nr", "--script", script, "--project-dir", dir, "--config-dir", cfg, "-p", "again")
	assert.Equal(t, 0, code, "without it, the same resume succeeds; output:\n%s", out)
	assert.Equal(t, 2, strings.Count(readOrEmpty(runs), "run"))
}

// The Agent call's script field wins over A10N_MOCK_SUBAGENT_SCRIPT: with both
// set, the sub-agent runs the call's script and not the env's.
// sr:proves subagent-script
func TestT009_15_SubagentScriptFieldWinsOverEnv(t *testing.T) {
	dir := t.TempDir()
	fromField, fromEnv := filepath.Join(dir, "field-ran"), filepath.Join(dir, "env-ran")
	sub := func(name, marker string) string {
		return writeScript(t, dir, name, "#!/bin/sh\n: > \""+marker+"\"\nprintf '%s\\n' '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"SUB\",\"is_error\":false}'\n")
	}
	field, env := sub("field.sh", fromField), sub("env.sh", fromEnv)
	script := writeScript(t, dir, "s.sh", `#!/bin/sh
if grep -q 'agentId:' "$A10N_MOCK_SESSION_FILE"; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"a1","name":"Agent","input":{"description":"d","prompt":"p","subagent_type":"general-purpose","script":"`+field+`"}}]}}'
`)
	out, code := runInDir(t, dir, []string{"A10N_MOCK_SUBAGENT_SCRIPT=" + env}, "--script", script, "--session-id", "s-sw", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.True(t, fileExists(fromField), "the call's script ran")
	assert.False(t, fileExists(fromEnv), "the env's did not")
}
