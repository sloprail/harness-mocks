package replay

import (
	"time"
)

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

// descendants calls visit for the agent and for every sub-agent below it, each with the positions
// (among the sub-agents each started) that lead down to it from the agent: none for the agent itself.
func descendants(a *Agent, via []int, visit func(sub *Agent, via []int)) {
	visit(a, via)
	m := 0
	for _, c := range a.Calls {
		if c.Tool != ToolSpawn || c.Sub == nil {
			continue
		}
		descendants(c.Sub, append(append([]int(nil), via...), m), visit)
		m++
	}
}
