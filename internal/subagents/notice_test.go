package subagents

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/sloprail/harness-mocks/internal/tasks"
)

// An owner is told of its ended sub-agents one at a time, in start order, once; not of one still
// running, a sub-agent of another owner or a command.
func TestNoticesAreTheOwnersEndedSubAgentsOneAtATime(t *testing.T) {
	reg := tasks.NewRegistry()
	defer reg.Shutdown()
	for _, c := range []struct{ id, owner, answer string }{{"a", "me", "A!"}, {"b", "other", "B!"}, {"c", "me", "C!"}} {
		task := tasks.NewTask(tasks.Agent, c.id)
		task.Owner = c.owner
		reg.StartAgent(task, func(context.Context) { task.Result = c.answer })
	}
	running := tasks.NewTask(tasks.Agent, "r")
	running.Owner = "me"
	reg.StartAgent(running, func(ctx context.Context) { <-ctx.Done() })
	time.Sleep(100 * time.Millisecond)
	n, ok := TakeNotice(reg, "me")
	assert.True(t, ok)
	assert.Equal(t, Notice{ID: "a", Answer: "A!"}, n)
	n, ok = TakeNotice(reg, "me")
	assert.True(t, ok)
	assert.Equal(t, Notice{ID: "c", Answer: "C!"}, n)
	_, ok = TakeNotice(reg, "me")
	assert.False(t, ok, "once, and none still running")
}
