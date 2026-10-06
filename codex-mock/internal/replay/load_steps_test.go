package replay

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	core "github.com/sloprail/harness-mocks/internal/replay"
	"github.com/sloprail/harness-mocks/internal/scenario"
)

func user(text string) map[string]any {
	return map[string]any{"type": "response_item", "payload": map[string]any{"type": "message", "role": "user",
		"content": []any{map[string]any{"type": "input_text", "text": text}}}}
}

func marker(name string) map[string]any {
	return map[string]any{"type": "response_item", "payload": map[string]any{"type": "marker", "name": name}}
}

// A thread worked in twice holds the runs one after the other, each after its own prompt; a
// forked thread starts with a copy of the history, and its run is what follows its own prompt.
func TestStepRecordsSplitsAResumedThreadAndAForkedOne(t *testing.T) {
	specs := []stepSpec{{prompt: "first"}, {prompt: "ask", args: []string{"fork", "<SESSION>"}}, {prompt: "again", args: []string{"resume", "<SESSION>"}}}
	threads := []string{"A", "B", "A"}
	rollouts := map[string][]map[string]any{
		"A": {user("first"), marker("a1"), user("again"), marker("a2")},
		"B": {user("first"), marker("a1"), user("ask"), marker("b1")},
	}
	got, err := stepRecords(specs, threads, rollouts)
	require.NoError(t, err)
	names := func(rs []map[string]any) (out []string) {
		for _, r := range rs {
			if p := r["payload"].(map[string]any); p["type"] == "marker" {
				out = append(out, p["name"].(string))
			}
		}
		return
	}
	assert.Equal(t, []string{"a1"}, names(got[0]), "the first run ends where the resumed one's prompt is")
	assert.Equal(t, []string{"b1"}, names(got[1]), "the fork's run is after its own prompt, not the copied history")
	assert.Equal(t, []string{"a2"}, names(got[2]))

	_, err = stepRecords(specs, threads, map[string][]map[string]any{"A": rollouts["A"], "B": {user("first")}})
	assert.Error(t, err, "a run whose prompt is not in its thread's rollout is not guessed")
	_, err = stepRecords(specs, threads[:2], rollouts)
	assert.Error(t, err, "a run the stream does not show")
}

// A later run's script starts after the steps its session already holds: a resume continues the
// first session, a fork starts from a copy of it, so a fork is not offset by what a resume did.
func TestThenScriptsStartAfterTheStepsTheirSessionHolds(t *testing.T) {
	call := func(c string) core.Call {
		return core.Call{Tool: core.ToolShell, Input: map[string]any{"command": c}}
	}
	rec := core.Recording{
		Agent: core.Agent{Calls: []core.Call{call("a"), call("b")}, Final: "OK"},
		Then: []core.Step{
			{Prompt: "p1", Args: []string{"resume", "<SESSION>"}, Agent: core.Agent{Calls: []core.Call{call("c")}, Final: "R1"}},
			{Prompt: "p2", Args: []string{"fork", "<SESSION>"}, Agent: core.Agent{Final: "F"}},
			{Prompt: "p3", Args: []string{"resume", "<SESSION>"}, Agent: core.Agent{Final: "R2"}},
		},
	}
	then := thenScenario(rec)
	require.Len(t, then, 3)
	for i, base := range []int{2, 3, 3} {
		assert.Contains(t, then[i].Script, `sed -n "$((n+k+1-`+strconv.Itoa(base)+`))p"`, "run %d", i+1)
	}
	assert.True(t, strings.Contains(then[0].Script, "call_then1_"))
}

// A compaction the harness made is a step of the model's turns the script repeats at the same place,
// and the script counts the compactions among the steps it has taken.
func TestACompactedRecordIsACompactStepOfTheScript(t *testing.T) {
	agent, err := modelTurns([]map[string]any{{"type": "compacted", "payload": map[string]any{}}}, nil)
	require.NoError(t, err)
	require.Len(t, agent.Calls, 1)
	assert.Equal(t, core.ToolCompact, agent.Calls[0].Tool)

	script := scriptFor("main", 0, []modelCall{mockCall(agent.Calls[0])}, "DONE", false, scenario.Gate{})
	assert.Contains(t, script, `{"type":"compact","trigger":"auto"}`)
	assert.Contains(t, script, `grep -c '"type":"compacted"'`, "a compaction spends a step")
}
