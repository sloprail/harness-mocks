package runner

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// A yield time of zero is a yield time (recorded: runs/task-notifications-bg):
// the call returns at once. No yield time is no yield.
func TestYieldTime(t *testing.T) {
	for in, want := range map[string]struct {
		d time.Duration
		y bool
	}{
		`{"command":"x"}`:                      {0, false},
		`{"command":"x","yield_time_ms":0}`:    {0, true},
		`{"command":"x","yield_time_ms":1500}`: {1500 * time.Millisecond, true},
	} {
		d, y := yieldTime(toolcall.Call{Input: []byte(in)})
		assert.Equal(t, want.d, d, in)
		assert.Equal(t, want.y, y, in)
	}
}
