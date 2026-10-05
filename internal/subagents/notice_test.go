package subagents

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/sloprail/harness-mocks/internal/tasks"
)

// An owner is told of each sub-agent it started that has ended, once, in start order;
// not of one still running, a sub-agent of another owner or a command.
func TestNoticesAreTheOwnersEndedSubAgentsOnce(t *testing.T) {
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
	assert.Equal(t, []Notice{{ID: "a", Answer: "A!"}, {ID: "c", Answer: "C!"}}, TakeNotices(reg, "me"))
	assert.Empty(t, TakeNotices(reg, "me"), "once")
}
