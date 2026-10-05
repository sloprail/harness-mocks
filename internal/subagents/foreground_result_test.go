package subagents

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sloprail/harness-mocks/internal/tasks"
)

// start registers a sub-agent that ends after d with a report.
func start(reg *tasks.Registry, id, report string, d time.Duration) {
	t := tasks.NewTask(tasks.Agent, id)
	reg.StartAgent(t, func(ctx context.Context) {
		select {
		case <-time.After(d):
			t.Result = report
		case <-ctx.Done():
		}
	})
}

func TestWaitReturnsAtTheFirstToFinish(t *testing.T) {
	reg := tasks.NewRegistry()
	defer reg.Shutdown()
	start(reg, "a", "A!", time.Hour)
	start(reg, "b", "B!", 50*time.Millisecond)
	res, err := Wait(context.Background(), reg, []string{"a", "b", "zz"}, 5*time.Second)
	require.NoError(t, err)
	assert.False(t, res.TimedOut)
	assert.Equal(t, []WaitState{{"a", WaitRunning, ""}, {"b", WaitCompleted, "B!"}, {"zz", WaitNotFound, ""}}, res.States)
}

func TestWaitTimesOutWithAnEmptyStatus(t *testing.T) {
	reg := tasks.NewRegistry()
	defer reg.Shutdown()
	start(reg, "a", "", time.Hour)
	res, err := Wait(context.Background(), reg, []string{"a"}, 30*time.Millisecond)
	require.NoError(t, err)
	assert.True(t, res.TimedOut)
	assert.Empty(t, res.States)
}

func TestWaitOnNoKnownSubAgentReturnsAtOnce(t *testing.T) {
	reg := tasks.NewRegistry()
	defer reg.Shutdown()
	res, err := Wait(context.Background(), reg, []string{"x"}, time.Hour)
	require.NoError(t, err)
	assert.False(t, res.TimedOut)
	assert.Equal(t, WaitNotFound, res.States[0].Status)
}

func TestAFinishedSubAgentIsStillToldOf(t *testing.T) {
	reg := tasks.NewRegistry()
	defer reg.Shutdown()
	start(reg, "a", "A!", 0)
	time.Sleep(50 * time.Millisecond)
	res, err := Wait(context.Background(), reg, []string{"a"}, time.Second)
	require.NoError(t, err)
	assert.Equal(t, []WaitState{{"a", WaitCompleted, "A!"}}, res.States)
}

func TestTimeoutIsClamped(t *testing.T) {
	five, huge := 5, 99999999
	assert.Equal(t, 30*time.Second, Timeout(nil, 30000, 10000, 3600000))
	assert.Equal(t, 10*time.Second, Timeout(&five, 30000, 10000, 3600000))
	assert.Equal(t, time.Hour, Timeout(&huge, 30000, 10000, 3600000))
}
