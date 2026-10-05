package replay

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// playedBy runs the generated script against a session file that holds the
// given numbers of answered calls and hook continuations, and says what it plays.
func playedBy(t *testing.T, script string, answered, continued int) string {
	t.Helper()
	dir := t.TempDir()
	session := filepath.Join(dir, "session.jsonl")
	body := strings.Repeat(`{"payload":{"type":"function_call_output","output":"x"}}`+"\n", answered) +
		strings.Repeat(`{"payload":{"type":"message","content":"<hook_prompt>x</hook_prompt>"}}`+"\n", continued)
	require.NoError(t, os.WriteFile(session, []byte(body), 0o644))
	path := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	cmd := exec.Command("sh", path)
	cmd.Env = append(os.Environ(), "A10N_MOCK_SESSION_FILE="+session)
	out, err := cmd.Output()
	require.NoError(t, err)
	return string(out)
}

// The step played is the number of calls answered plus the hook continuations
// seen; past the last step the last one is played again.
func TestScriptPlaysTheStepNumberedByAnswersAndContinuations(t *testing.T) {
	rec := core.Recording{Agent: core.Agent{Calls: []core.Call{
		answer("FIRST"),
		{Tool: core.ToolShell, Input: map[string]any{"command": "echo a"}},
		answer("MID"),
	}, Final: "LAST"}}
	script := Denormalize(rec).Script
	for _, c := range []struct {
		answered, continued int
		want                string
	}{
		{0, 0, "FIRST"}, {0, 1, "echo a"}, {1, 0, "echo a"}, {1, 1, "MID"}, {1, 2, "LAST"}, {5, 5, "LAST"},
	} {
		assert.Contains(t, playedBy(t, script, c.answered, c.continued), c.want, "%d answered, %d continued", c.answered, c.continued)
	}
}

func rolloutOf(items ...map[string]any) (out []map[string]any) {
	for _, p := range items {
		out = append(out, map[string]any{"type": "response_item", "payload": p})
	}
	return out
}

func say(phase, text string) map[string]any {
	return map[string]any{"type": "message", "role": "assistant", "phase": phase,
		"content": []any{map[string]any{"text": text}}}
}

func exec1(cmd string) map[string]any {
	return map[string]any{"type": "custom_tool_call", "name": "exec", "input": `await tools.exec_command({cmd:"` + cmd + `"});`}
}

// The last answer of a rollout is the agent's final one; the others are steps
// among its calls, and what the model said before one is not carried past it.
func TestModelTurnsKeepsAnswersAmongTheCalls(t *testing.T) {
	agent, err := modelTurns(rolloutOf(say("final_answer", "A1"), say("commentary", "going"), exec1("echo a"), say("final_answer", "A2"), say("final_answer", "A3")))
	require.NoError(t, err)
	assert.Equal(t, "A3", agent.Final)
	require.Len(t, agent.Calls, 3)
	assert.Equal(t, core.ToolAnswer, agent.Calls[0].Tool)
	assert.Equal(t, core.ToolShell, agent.Calls[1].Tool)
	assert.Equal(t, "going", *agent.Calls[1].Said)
	assert.Equal(t, "A2", agent.Calls[2].Input["text"])

	agent, err = modelTurns(rolloutOf(exec1("echo a")))
	require.NoError(t, err)
	assert.Equal(t, "", agent.Final)
	assert.Len(t, agent.Calls, 1)
}
