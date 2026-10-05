package replay

import (
	"fmt"
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
	return map[string]any{"type": "custom_tool_call", "name": "exec", "call_id": "id-" + cmd, "input": `await tools.exec_command({cmd:"` + cmd + `"});`}
}

// answered is the recorded output of exec1's script.
func answered(cmd string) map[string]any {
	return map[string]any{"type": "custom_tool_call_output", "call_id": "id-" + cmd, "output": "Script completed"}
}

// The last answer of a rollout is the agent's final one; the others are steps
// among its calls, and what the model said before one is not carried past it.
func TestModelTurnsKeepsAnswersAmongTheCalls(t *testing.T) {
	agent, err := modelTurns(rolloutOf(say("final_answer", "A1"), say("commentary", "going"), exec1("echo a"), answered("echo a"), say("final_answer", "A2"), say("final_answer", "A3")), nil)
	require.NoError(t, err)
	assert.Equal(t, "A3", agent.Final)
	require.Len(t, agent.Calls, 3)
	assert.Equal(t, core.ToolAnswer, agent.Calls[0].Tool)
	assert.Equal(t, core.ToolShell, agent.Calls[1].Tool)
	assert.Equal(t, "going", *agent.Calls[1].Said)
	assert.Equal(t, "A2", agent.Calls[2].Input["text"])

	agent, err = modelTurns(rolloutOf(exec1("echo a"), answered("echo a")), nil)
	require.NoError(t, err)
	assert.Equal(t, "", agent.Final)
	assert.Len(t, agent.Calls, 1)
}

// What a script's recorded output says is held against the calls read out of it.
func TestModelTurnsHoldsOutputsAgainstTheDerivedCalls(t *testing.T) {
	two := map[string]any{"type": "custom_tool_call", "name": "exec", "call_id": "two", "input": `await tools.exec_command({cmd:"a"}); await tools.exec_command({cmd:"b"});`}
	failed := map[string]any{"type": "custom_tool_call_output", "call_id": "two", "output": []any{map[string]any{"text": "Script failed\nOutput:"}}}
	_, err := modelTurns(rolloutOf(two, failed), nil)
	require.Error(t, err, "which of two calls ran is not recorded")
	_, err = modelTurns(rolloutOf(exec1("echo a")), nil)
	require.Error(t, err, "a script with no output")
	_, err = modelTurns(rolloutOf(answered("echo a")), nil)
	require.Error(t, err, "an output of no script")
	_, err = modelTurns(rolloutOf(say("commentary", "one"), say("commentary", "two"), exec1("echo a"), answered("echo a")), nil)
	require.Error(t, err, "the first commentary would be lost")
}

func TestSpawnReceiptsAreTheMainThreadsCompletedSpawns(t *testing.T) {
	item := func(typ, sender string, receivers ...any) map[string]any {
		return map[string]any{"type": typ, "item": map[string]any{"type": "collab_tool_call", "tool": "spawn_agent", "sender_thread_id": sender, "receiver_thread_ids": receivers}}
	}
	got := spawnReceipts([]map[string]any{item("item.started", "m"), item("item.completed", "m", "a"), item("item.completed", "x", "n"), item("item.completed", "m"), item("item.completed", "m", "b")}, "m")
	assert.Equal(t, []string{"a", "b"}, got)
}

// A wait's targets are the receipts of the agent's spawns, put in by position;
// no text of the script is rewritten (a literal AGENT1 or IDPLACE stays), and
// there is no limit of nine spawns.
func TestWaitTargetsAreFilledInStructurally(t *testing.T) {
	said := "AGENT1 IDPLACE"
	rec := core.Recording{Agent: core.Agent{Calls: []core.Call{
		{Tool: core.ToolWait, Said: &said, Input: map[string]any{"targets": []int{0, 10}, "timeout_ms": 5}},
	}}}
	script := Denormalize(rec).Script
	dir := t.TempDir()
	session := filepath.Join(dir, "session.jsonl")
	var lines []string
	for i := 0; i < 11; i++ {
		lines = append(lines, fmt.Sprintf(`{"payload":{"type":"function_call_output","output":"{\"agent_id\":\"id-%d\"}"}}`, i))
	}
	// the wait is the first step whatever the session holds: the step number is pinned to 0
	require.NoError(t, os.WriteFile(session, []byte(strings.Join(lines, "\n")+"\n"), 0o644))
	path := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(path, []byte(strings.Replace(script, `n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")`, `n=0`, 1)), 0o755))
	cmd := exec.Command("sh", path)
	cmd.Env = append(os.Environ(), "A10N_MOCK_SESSION_FILE="+session)
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), `"targets":["id-0","id-10"]`)
	assert.Contains(t, string(out), "AGENT1 IDPLACE")
}

// A wait for a spawn that has no receipt in the session stops the script loudly.
func TestWaitForAMissingReceiptFailsTheScript(t *testing.T) {
	rec := core.Recording{Agent: core.Agent{Calls: []core.Call{
		{Tool: core.ToolWait, Input: map[string]any{"targets": []int{3}, "timeout_ms": 5}},
	}}}
	script := strings.Replace(Denormalize(rec).Script, `n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")`, `n=0`, 1)
	dir := t.TempDir()
	session := filepath.Join(dir, "session.jsonl")
	require.NoError(t, os.WriteFile(session, []byte(`{"payload":{"type":"function_call_output","output":"{\"agent_id\":\"id-0\"}"}}`+"\n"), 0o644))
	path := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	cmd := exec.Command("sh", path)
	cmd.Env = append(os.Environ(), "A10N_MOCK_SESSION_FILE="+session)
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "no spawn receipt at position 3")
}

// An agent the recording shows still at work does not end in the replay: its script hangs
// instead of answering (recorded: runs/print-waits-for-background-agents, a sub-agent cut short).
func TestAnUnfinishedAgentsScriptHangs(t *testing.T) {
	sub := &core.Agent{Unfinished: true}
	rec := core.Recording{Agent: core.Agent{Calls: []core.Call{{Tool: core.ToolSpawn, Input: map[string]any{"message": "m"}, Sub: sub}}}}
	s := Denormalize(rec)
	assert.Contains(t, s.Files["sub0.sh"]+s.Scripts["sub0.sh"], "{\"hang\":true}\nSTEPS_EOF")
	assert.NotContains(t, s.Script, "{\"hang\":true}\nSTEPS_EOF")
}
