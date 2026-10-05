package replay

import (
	"time"

	core "github.com/sloprail/harness-mocks/internal/replay"
	"github.com/sloprail/harness-mocks/internal/scenario"
)

// gatesOf are the gates the script puts on each step of an agent (its calls and
// answers in order, then its final answer): what must have happened before the mock
// takes the step, as the recording's times show it happened. The order of the agents'
// steps is the script's then, and no delay decides it.
//
//   - A step of the agent waits for the sub-agents that had ended before it was taken
//     and after the step before it: the first step taken after a sub-agent's final answer.
//   - A step of a sub-agent waits for the agent that started it to have started and
//     finished as many calls as the recording shows it had by then.
func gatesOf(a core.Agent, parent *core.Agent) []scenario.Gate {
	steps := len(a.Calls) + 1
	out := make([]scenario.Gate, steps)
	at := func(j int) time.Time {
		if j < len(a.Calls) {
			return a.Calls[j].At
		}
		return a.FinalAt
	}
	k := 0
	for i, c := range a.Calls {
		if c.Tool != core.ToolSpawn || c.Sub == nil {
			continue
		}
		if end := c.Sub.FinalAt; !c.Sub.Unfinished && !end.IsZero() {
			for j := i + 1; j < steps; j++ {
				if at(j).After(end) {
					out[j].Ended = append(out[j].Ended, k)
					break
				}
			}
		}
		k++
	}
	if parent != nil {
		var started, done int
		for j := range out {
			t := at(j)
			if t.IsZero() {
				continue
			}
			s, d := progressBy(*parent, t)
			if s > started {
				out[j].ParentStarted, started = s, s
			}
			if d > done {
				out[j].ParentDone, done = d, d
			}
		}
	}
	return out
}

// progressBy is how many calls of the agent had been started, and how many finished, before t.
func progressBy(a core.Agent, t time.Time) (started, done int) {
	for _, c := range a.Calls {
		if c.Tool == core.ToolAnswer {
			continue
		}
		if !c.At.IsZero() && c.At.Before(t) {
			started++
		}
		if !c.Done.IsZero() && c.Done.Before(t) {
			done++
		}
	}
	return
}
