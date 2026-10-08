package runner

import "github.com/sloprail/harness-mocks/internal/scenario"

// Order is the calls of a response in the order Cursor was recorded taking them:
// a sub-agent's Task last, whatever the model listed it as (recorded:
// runs/task-stream-frames and runs/nested-subagents-background, each a response of
// a Task and another call whose frame comes first; no other recording has a
// response of several calls beside them).
func (s *session) Order(calls []scenario.ToolUse) []scenario.ToolUse {
	for _, c := range calls {
		s.batched.Store(c.ID, true)
	}
	var tasks, rest []scenario.ToolUse
	for _, c := range calls {
		if c.Name == "Task" || c.Name == "Agent" {
			tasks = append(tasks, c)
		} else {
			rest = append(rest, c)
		}
	}
	return append(rest, tasks...)
}

// Waits is turnloop.Waiter: a wait (AwaitShell) outlasts the other calls of its
// response, which are in flight beside it (recorded: runs/nested-subagents-background,
// where the wait of 60 s is completed after the sub-agent the same response started).
func (s *session) Waits(tu scenario.ToolUse) bool { return tu.Name == "AwaitShell" }
