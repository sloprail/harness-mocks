package replay

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// No more replays hold a slot at once than the gate is wide.
func TestGateLimitsWhoHoldsASlot(t *testing.T) {
	var in, most atomic.Int32
	done := make(chan struct{})
	for range 3 * cap(replays) {
		go func() {
			defer func() { done <- struct{}{} }()
			defer hold()()
			n := in.Add(1)
			for m := most.Load(); n > m && !most.CompareAndSwap(m, n); m = most.Load() {
			}
			time.Sleep(10 * time.Millisecond)
			in.Add(-1)
		}()
	}
	for range 3 * cap(replays) {
		<-done
	}
	assert.LessOrEqual(t, int(most.Load()), cap(replays))
	assert.GreaterOrEqual(t, cap(replays), 1)
	assert.LessOrEqual(t, cap(replays), 4)
}
