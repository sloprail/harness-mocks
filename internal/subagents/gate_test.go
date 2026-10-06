package subagents

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/sloprail/harness-mocks/internal/scenario"
	"github.com/sloprail/harness-mocks/internal/tasks"
)

// A gate holds a step until what the script names has happened: the parent has started and
// finished as many calls as it says, and the sub-agents it names have ended. Nothing is waited
// for by time: each wait is released by the event it names.
func TestAGateIsReleasedByTheEventsItNames(t *testing.T) {
	reg := tasks.NewRegistry()
	defer reg.Shutdown()
	parent := NewProgress()
	spawned := &SpawnLog{}
	task := tasks.NewTask(tasks.Agent, "kid")
	release := make(chan struct{})
	reg.StartAgent(task, func(ctx context.Context) { <-release })
	spawned.Add("kid")

	held := make(chan []string, 1)
	go func() {
		held <- Hold(context.Background(), scenario.Gate{ParentStarted: 2, ParentDone: 1, Ended: []int{0}}, reg, spawned, parent)
	}()
	released := func() ([]string, bool) {
		select {
		case p := <-held:
			return p, true
		case <-time.After(50 * time.Millisecond):
			return nil, false
		}
	}
	_, ok := released()
	assert.False(t, ok, "nothing has happened yet")
	parent.Move(2, 0)
	_, ok = released()
	assert.False(t, ok, "started, not finished")
	parent.Move(0, 1)
	_, ok = released()
	assert.False(t, ok, "the sub-agent has not ended")
	close(release)
	problems, ok := released()
	assert.True(t, ok)
	assert.Empty(t, problems)
}

// A gate that names what does not exist is the script's mistake: it is reported, not waited for.
func TestAGateNamingWhatDoesNotExistIsReported(t *testing.T) {
	problems := Hold(context.Background(), scenario.Gate{Ended: []int{3}, ParentStarted: 1}, tasks.NewRegistry(), &SpawnLog{}, nil)
	assert.Len(t, problems, 2)
	assert.Contains(t, problems[0], "started by none")
	assert.Contains(t, problems[1], "sub-agent 3, which this agent has not started")
}
