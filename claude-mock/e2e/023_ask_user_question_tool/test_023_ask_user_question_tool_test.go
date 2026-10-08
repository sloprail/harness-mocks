package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const questions = `[{"question":"Which colour?","header":"Colour","options":[{"label":"Red","description":"warm"},{"label":"Blue","description":"cool"}],"multiSelect":false},{"question":"Which sizes?","header":"Sizes","options":[{"label":"S","description":"small"},{"label":"M","description":"medium"}],"multiSelect":true}]`

// ask runs a script that calls AskUserQuestion once with the questions above. answers is the object a
// PreToolUse hook gives back as the answers in its updatedInput ("" when no hook answers). It returns the
// run's exit code and output, the hook's log of the payloads it saw, and the session's transcript.
func ask(t *testing.T, answers string, extra ...string) (code int, out, hookLog, transcript string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "hooks.log")
	hook := filepath.Join(dir, "hook.sh")
	reply := ""
	if answers != "" {
		reply = `if grep -q '"hook_event_name":"PreToolUse"' "$f"; then printf '%s' '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","updatedInput":{"questions":` + questions + `,"answers":` + answers + `}}}'; fi`
	}
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\nf="+filepath.Join(dir, "in.json")+"\ncat > \"$f\"\ncat \"$f\" >> "+log+"\necho >> "+log+"\n"+reply+"\n"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"),
		[]byte(`{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"`+hook+`"}]}],"PostToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"`+hook+`"}]}]}}`), 0o644))
	script := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
if [ -n "$A10N_MOCK_SESSION_FILE" ] && grep -q "tool_result" "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":null,"content":[{"type":"tool_use","id":"aq1","name":"AskUserQuestion","input":{"questions":`+questions+`}}]}}'
`), 0o755))
	args := append([]string{"--script", script, "--session-id", "ask-1", "--project-dir", dir, "--config-dir", filepath.Join(dir, ".cfg"), "-p"}, extra...)
	out, code = runInDir(t, dir, nil, append(args, "go")...)
	data, _ := os.ReadFile(log)
	files, _ := filepath.Glob(filepath.Join(dir, ".cfg", "projects", "*", "ask-1.jsonl"))
	var recorded []byte
	if len(files) == 1 {
		recorded, _ = os.ReadFile(files[0])
	}
	return code, out, string(data), string(recorded)
}

// TestT023_01_TheAnswersOfAHookReachTheAgent: a PreToolUse hook that allows the call with an updatedInput holding
// the questions and their answers answers it: the agent is told its questions "have been answered" when every
// answer is an option's label (a multiple-choice one's labels joined by a comma and a space), the result is the
// input with the answers in it, PostToolUse sees that input, and the transcript keeps the call as the model made
// it (recorded: runs/ask-user-question-tool, step 1).
// sr:proves ask-user-question-tool/claude
func TestT023_01_TheAnswersOfAHookReachTheAgent(t *testing.T) {
	code, out, hooks, transcript := ask(t, `{"Which colour?":"Blue","Which sizes?":"S, M"}`, "--permission-prompt-tool", "stdio")
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `"content":"Your questions have been answered: \"Which colour?\"=\"Blue\", \"Which sizes?\"=\"S, M\". You can now continue with these answers in mind."`)
	assert.Contains(t, out, `"tool_use_result":{"answers":{"Which colour?":"Blue","Which sizes?":"S, M"},"questions":[`)
	assert.NotContains(t, out, `"is_error":true`)
	assert.Contains(t, hooks, `"hook_event_name":"PostToolUse","tool_name":"AskUserQuestion"`)
	assert.Contains(t, hooks, `"answers":{"Which colour?":"Blue","Which sizes?":"S, M"}`)
	for _, line := range strings.Split(hooks, "\n") {
		if strings.Contains(line, `"hook_event_name":"PreToolUse"`) {
			assert.NotContains(t, line, `"answers"`, "PreToolUse saw the call as the model made it")
		}
	}
	for _, line := range strings.Split(transcript, "\n") {
		if strings.Contains(line, `"type":"tool_use"`) {
			assert.NotContains(t, line, `"answers"`, "the transcript keeps the call as the model made it")
		}
	}
}

// TestT023_02_AnswerInTheUsersOwnWordsIsToldNeutrally: when any answer is no option's label, the agent is told "The
// user answered:" and to read the answers carefully, rather than that its questions "have been answered" (recorded:
// runs/ask-user-question-tool, steps 2 and 3).
// sr:proves ask-user-question-tool/claude
func TestT023_02_AnswerInTheUsersOwnWordsIsToldNeutrally(t *testing.T) {
	code, out, _, _ := ask(t, `{"Which colour?":"Blue","Which sizes?":"S, XL"}`, "--permission-prompt-tool", "stdio")
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `"content":"The user answered: \"Which colour?\"=\"Blue\", \"Which sizes?\"=\"S, XL\". Read the answers carefully — they may request clarification, changes, or that you not proceed — and follow what they actually say."`)
	code, out, _, _ = ask(t, `{"Which colour?":"teal","Which sizes?":"S"}`, "--permission-prompt-tool", "stdio")
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `The user answered: \"Which colour?\"=\"teal\", \"Which sizes?\"=\"S\". Read the answers carefully`)
}

// TestT023_03_AQuestionNoOneAnsweredIsAnError: with no hook to answer it, the call is an error result saying that the
// mock has no user to ask.
func TestT023_03_AQuestionNoOneAnsweredIsAnError(t *testing.T) {
	code, out, _, _ := ask(t, "", "--permission-prompt-tool", "stdio")
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `"is_error":true`)
	assert.Contains(t, out, "the mock has no user to ask")
}

// TestT023_04_AnonInteractiveRunWithoutAPermissionHostHasNoSuchTool: AskUserQuestion is offered to a print run only
// when it has a permission host (--permission-prompt-tool stdio); without one a call to it fails the run, and a host
// other than stdio is refused (adr/fail-fast-unimplemented).
func TestT023_04_AnonInteractiveRunWithoutAPermissionHostHasNoSuchTool(t *testing.T) {
	code, out, _, _ := ask(t, `{"Which colour?":"Blue","Which sizes?":"S"}`)
	require.NotZero(t, code, out)
	assert.Contains(t, out, "permission host")
	code, out, _, _ = ask(t, "", "--permission-prompt-tool", "mcp__host__ask")
	require.NotZero(t, code, out)
	assert.Contains(t, out, "--permission-prompt-tool mcp__host__ask is not implemented by the mock")
}
