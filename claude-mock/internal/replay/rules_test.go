package replay

import (
	"strings"
	"testing"

	rp "github.com/sloprail/harness-mocks/internal/replay"
)

// A scheduled wakeup's wall-clock time and seconds-to-go, and when it falls, are the run's own: the
// text and the key are compared, their values are not (recorded: runs/schedule-wakeup-limits).
func TestWakeupTimesAreMasked(t *testing.T) {
	c := rp.New(Rules("/r", "/w", nil))
	got := c.Lines([]map[string]any{{"content": "Next wakeup scheduled for 16:19:00 (in 120s) (clamped to 60s)", "scheduledFor": 1790864340000.0}})
	line := strings.NewReplacer(`\u003c`, "<", `\u003e`, ">").Replace(got[0])
	want := `"content":"Next wakeup scheduled for <TIME> (in <N>s) (clamped to 60s)"`
	if len(got) != 1 || !strings.Contains(line, want) || !strings.Contains(line, `"scheduledFor":"`+rp.MaskedValue+`"`) {
		t.Fatalf("%v", got)
	}
}
