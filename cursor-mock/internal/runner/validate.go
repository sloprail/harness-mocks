package runner

import (
	"sync"

	"github.com/sloprail/harness-mocks/internal/toolspec"
)

// refusalSets holds each run's refusals, by the run's main session (the one with
// no parent): its sub-agents' sessions are copies of their parents', so the chain
// of parents ends at it, the same way a run's background shells are found
// (registry).
var refusalSets sync.Map // *session -> *toolspec.Refusals

// Tools is the schema every call of a script is checked against before the
// session is given it (turnloop.Validator).
func (s *session) Tools() toolspec.Schema { return schema }

// Refusals is where a refused call of the run is kept, so the run ends with it
// whichever agent's script asked (turnloop.Validator).
func (s *session) Refusals() *toolspec.Refusals {
	root := s
	for root.parent != nil {
		root = root.parent
	}
	r, _ := refusalSets.LoadOrStore(root, &toolspec.Refusals{})
	return r.(*toolspec.Refusals)
}
