package runner

import (
	"context"
	"strings"
	"sync"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
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

// refuseResult refuses the run when a tool's result is the mock's refusal.
func (s *session) refuseResult(r toolexec.Result) {
	if msg, ok := r.NotModeled(); ok {
		s.refusal.refuse(msg)
	}
}

// refuseMsg refuses the run when msg is the mock's refusal, and returns it.
func (s *session) refuseMsg(msg string) string {
	if strings.HasPrefix(msg, toolexec.NotModeledPrefix) {
		s.refusal.refuse(msg)
	}
	return msg
}

func (r *refusal) message() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.msg
}
