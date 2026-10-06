package replay

import (
	"time"

	"github.com/sloprail/harness-mocks/internal/scenario"
)

// Gates are the gates the script puts on each step of an agent (its calls and
// answers in order, then its final answer): what must have happened before the mock
// takes the step, as the recording's times show it happened. The order of the agents'
// steps is the script's then, and no delay decides it.
//
//   - A step of the agent waits for the sub-agents that had ended before it was taken
//     and after the step before it: the first step taken after a sub-agent's final answer.
//   - A step of a sub-agent waits for the agent that started it to have started and
//     finished as many calls as the recording shows it had by then.
func Gates(a Agent, parent *Agent, ancestors ...*Agent) []scenario.Gate {
	steps := len(a.Calls) + 1
	out := make([]scenario.Gate, steps)
	at := func(j int) time.Time {
		if j < len(a.Calls) {
			return a.Calls[j].At
		}
		return a.FinalAt
	}
	k := 0
	wanted := map[int]int{} // calls of each sub-agent a step is already told to wait for
	for i, c := range a.Calls {
		if c.Tool != ToolSpawn || c.Sub == nil {
			continue
		}
		// A step waits for the calls of a sub-agent that it had started by then, so that the two agents'
		// frames come in the order they came: the sub-agent was ahead of this step, whatever it took.
		for j := i + 1; j < steps; j++ {
			t := at(j)
			if t.IsZero() {
				continue
			}
			started := 0
			for _, sc := range c.Sub.Calls {
				if sc.Tool != ToolAnswer && !sc.At.IsZero() && sc.At.Before(t) {
					started++
				}
			}
			if started > wanted[k] && (c.Sub.Unfinished || c.Sub.FinalAt.IsZero() || c.Sub.FinalAt.After(t)) {
				out[j].ChildStarted = append(out[j].ChildStarted, scenario.ChildCalls{Sub: k, Calls: started})
				wanted[k] = started
			}
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
		ended := false
		for j := range out {
			if t := at(j); !ended && !t.IsZero() && !parent.Unfinished && !parent.FinalAt.IsZero() && parent.FinalAt.Before(t) {
				out[j].ParentEnded, ended = true, true // the agent that started this one had ended by then
			}
		}
		// A call of this agent that ran a while waits, before it is carried out, for the steps of the agent
		// that started it that came before the call's output was given back: they came while it ran.
		for j, c := range a.Calls {
			if c.Done.IsZero() || c.At.IsZero() {
				continue
			}
			if n := stepsBy(*parent, c.Done); n > stepsBy(*parent, c.At) {
				out[j].ExecParentSteps = n
			}
		}
		for i, anc := range ancestors { // and for the agents above that one
			for j, c := range a.Calls {
				if c.Done.IsZero() || c.At.IsZero() {
					continue
				}
				n := stepsBy(*anc, c.Done)
				for len(out[j].ExecAncestorSteps) <= i {
					out[j].ExecAncestorSteps = append(out[j].ExecAncestorSteps, 0)
				}
				if n > stepsBy(*anc, c.At) {
					out[j].ExecAncestorSteps[i] = n
				}
			}
		}
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
func progressBy(a Agent, t time.Time) (started, done int) {
	for _, c := range a.Calls {
		if c.Tool == ToolAnswer {
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

// stepsBy is how many steps the agent had taken before t: its calls started and its answers given.
func stepsBy(a Agent, t time.Time) (n int) {
	for _, c := range a.Calls {
		if !c.At.IsZero() && c.At.Before(t) {
			n++
		}
	}
	if !a.FinalAt.IsZero() && a.FinalAt.Before(t) {
		n++
	}
	return n
}
