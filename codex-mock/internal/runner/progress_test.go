package runner

import (
	"bytes"
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
	parent := newProgress()
	sub := &state{cfg: Config{Stderr: &bytes.Buffer{}}, bg: reg, parent: parent, spawned: &spawnLog{}}
	task := tasks.NewTask(tasks.Agent, "kid")
	release := make(chan struct{})
	reg.StartAgent(task, func(ctx context.Context) { <-release })
	sub.spawned.add("kid")

	held := make(chan struct{})
	go func() {
		sub.Gate(context.Background(), scenario.Gate{ParentStarted: 2, ParentDone: 1, Ended: []int{0}})
		close(held)
	}()
	released := func() bool {
		select {
		case <-held:
			return true
		case <-time.After(50 * time.Millisecond):
			return false
		}
	}
	assert.False(t, released(), "nothing has happened yet")
	parent.move(2, 0)
	assert.False(t, released(), "started, not finished")
	parent.move(0, 1)
	assert.False(t, released(), "the sub-agent has not ended")
	close(release)
	assert.True(t, released())
}

// A gate that names what does not exist is the script's mistake: it is reported, not waited for.
func TestAGateNamingWhatDoesNotExistIsReported(t *testing.T) {
	var stderr bytes.Buffer
	h := &state{cfg: Config{Stderr: &stderr}, bg: tasks.NewRegistry(), spawned: &spawnLog{}}
	h.Gate(context.Background(), scenario.Gate{Ended: []int{3}, ParentStarted: 1})
	assert.Contains(t, stderr.String(), "started by none")
	assert.Contains(t, stderr.String(), "sub-agent 3, which this agent has not started")
}
