package subagents

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func handle(done *atomic.Bool, report string) Handle {
	return Handle{Finished: done.Load, Report: func() string { return report }}
}

func TestWaitReturnsAtTheFirstToFinish(t *testing.T) {
	var w Waits
	var a, b atomic.Bool
	w.Add("a", handle(&a, "A!"))
	w.Add("b", handle(&b, "B!"))
	go func() { time.Sleep(50 * time.Millisecond); b.Store(true) }()
	res, err := w.Wait(context.Background(), []string{"a", "b", "zz"}, 5*time.Second)
	require.NoError(t, err)
	assert.False(t, res.TimedOut)
	assert.Equal(t, []WaitState{{"a", WaitRunning, ""}, {"b", WaitCompleted, "B!"}, {"zz", WaitNotFound, ""}}, res.States)
}

func TestWaitTimesOutWithAnEmptyStatus(t *testing.T) {
	var w Waits
	var a atomic.Bool
	w.Add("a", handle(&a, ""))
	res, err := w.Wait(context.Background(), []string{"a"}, 30*time.Millisecond)
	require.NoError(t, err)
	assert.True(t, res.TimedOut)
	assert.Empty(t, res.States)
}

func TestWaitOnNoKnownSubAgentReturnsAtOnce(t *testing.T) {
	var w Waits
	res, err := w.Wait(context.Background(), []string{"x"}, time.Hour)
	require.NoError(t, err)
	assert.False(t, res.TimedOut)
	assert.Equal(t, WaitNotFound, res.States[0].Status)
}

func TestTimeoutIsClamped(t *testing.T) {
	five, huge := 5, 99999999
	assert.Equal(t, 30*time.Second, Timeout(nil, 30000, 10000, 3600000))
	assert.Equal(t, 10*time.Second, Timeout(&five, 30000, 10000, 3600000))
	assert.Equal(t, time.Hour, Timeout(&huge, 30000, 10000, 3600000))
}
