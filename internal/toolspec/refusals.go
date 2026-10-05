package toolspec

import "sync"

// Refusals holds the first error a scenario script of a run ended with, above
// all a call it asked for that the mock does not implement. The sessions of a
// run's sub-agents share their parent's, so an error in a sub-agent's script
// fails the run, not only the sub-agent.
type Refusals struct {
	mu    sync.Mutex
	first error
}

// Set keeps err unless the run already holds an earlier one.
func (r *Refusals) Set(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.first == nil {
		r.first = err
	}
}

// Err is the first error kept, if any.
func (r *Refusals) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.first
}
