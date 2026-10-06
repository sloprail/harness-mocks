package runner

import (
	"context"
	"sync"
)

// refusal is the first refusal of something the mock does not model that a run
// met: a tool called in a way no recording shows, a flag, a model. It fails the
// run, so that a refusal inside a sub-agent, whose frames are not printed, cannot
// pass unseen. The mock's own refusals are worded "cursor-mock: ...".
type refusal struct {
	mu     sync.Mutex
	msg    string
	cancel context.CancelFunc
}

// refuse records the refusal and stops the run.
func (r *refusal) refuse(msg string) {
	r.mu.Lock()
	if r.msg == "" {
		r.msg = msg
	}
	r.mu.Unlock()
	r.cancel()
}

func (r *refusal) message() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.msg
}
