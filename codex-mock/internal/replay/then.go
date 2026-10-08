package replay

import (
	"fmt"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// thenScenario is the script of each later run of the harness. The session a run works in already
// holds the steps the earlier runs took (a resume continues the first session, a fork starts from
// a copy of it), so its script starts after them.
func thenScenario(rec core.Recording) []ThenStep {
	consumed := map[string]int{"first": len(rec.Agent.Calls)}
	var out []ThenStep
	for i, st := range rec.Then {
		thread := "first"
		if len(st.Args) > 0 && st.Args[0] == "fork" {
			thread = fmt.Sprintf("fork%d", i)
			consumed[thread] = consumed["first"]
		}
		calls := make([]modelCall, len(st.Agent.Calls))
		gates := core.Gates(st.Agent, nil)
		for j, c := range st.Agent.Calls {
			calls[j] = mockCall(c)
			calls[j].Gate = gates[j]
		}
		script := scriptFor(fmt.Sprintf("then%d", i+1), consumed[thread], calls, st.Agent.Final, st.Agent.Unfinished, gates[len(calls)])
		consumed[thread] += len(calls)
		out = append(out, ThenStep{Args: st.Args, Cwd: st.Cwd, Prompt: st.Prompt, Script: script})
	}
	return out
}
