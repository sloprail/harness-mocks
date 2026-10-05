package replay

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// No more replays hold a slot at once than the gate is wide.
func TestGateLimitsWhoHoldsASlot(t *testing.T) {
	g := NewGate()
	var in, most atomic.Int32
	done := make(chan struct{})
	for range 3 * cap(g) {
		go func() {
			defer func() { done <- struct{}{} }()
			defer g.Hold()()
			n := in.Add(1)
			for m := most.Load(); n > m && !most.CompareAndSwap(m, n); m = most.Load() {
			}
			time.Sleep(10 * time.Millisecond)
			in.Add(-1)
		}()
	}
	for range 3 * cap(g) {
		<-done
	}
	assert.LessOrEqual(t, int(most.Load()), cap(g))
	assert.GreaterOrEqual(t, cap(g), 1)
	assert.LessOrEqual(t, cap(g), 4)
}
