package scenario

import (
	"bytes"
	"encoding/json"
)

// Gate is a condition on other agents' progress, stated by the script on an assistant
// line (as "gate": {...}) and held by the host before it takes the line's calls and
// messages. Ended are the positions (in the order this agent started them) of the
// sub-agents that must have ended; ParentStarted and ParentDone are how many calls
// the agent that started this one must have started and have finished, and whether it must have ended. ChildStarted are
// sub-agents (by position) that must have started that many calls.
type Gate struct {
	Ended         []int `json:"ended,omitempty"`
	ParentStarted int   `json:"parent_started,omitempty"`
	ParentDone    int   `json:"parent_done,omitempty"`
	ParentEnded   bool  `json:"parent_ended,omitempty"`
	// ExecParentSteps is held before the step's call is carried out, not before it is taken: how many
	// steps (calls started, answers given) the agent that started this one must have taken by then.
	ExecParentSteps int `json:"exec_parent_steps,omitempty"`
	// ExecAncestorSteps is the same for the agents above that one, nearest first.
	ExecAncestorSteps []int        `json:"exec_ancestor_steps,omitempty"`
	ChildStarted      []ChildCalls `json:"child_started,omitempty"`
}

// ChildCalls is how many calls the sub-agent at position Sub must have started.
type ChildCalls struct {
	Sub   int `json:"sub"`
	Calls int `json:"calls"`
}

// UnmarshalJSON reads a gate and refuses a field it does not have: a script's mistake is never ignored.
func (g *Gate) UnmarshalJSON(b []byte) error {
	type plain Gate
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode((*plain)(g))
}

// None reports whether the gate holds nothing back.
func (g Gate) None() bool {
	return len(g.Ended) == 0 && g.ParentStarted == 0 && g.ParentDone == 0 && !g.ParentEnded && g.ExecParentSteps == 0 && len(g.ExecAncestorSteps) == 0 && len(g.ChildStarted) == 0
}
