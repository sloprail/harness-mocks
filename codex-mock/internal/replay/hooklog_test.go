package replay

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A line that is not JSON stays in the log in its place, and nothing is collapsed: two identical lines of
// a text-mode stdout, or of a hook log, are two.
func TestLogKeepsRawLinesAndCollapsesNothing(t *testing.T) {
	text := "{\"a\":1}\nhello\nhello\n{\"a\":1}\n{\"a\":1}\n"
	want := []map[string]any{{"a": float64(1)}, {"raw": "hello"}, {"raw": "hello"}, {"a": float64(1)}, {"a": float64(1)}}
	for _, parse := range []func(string) ([]map[string]any, error){parseHookLog, func(s string) ([]map[string]any, error) { return parseStream(s, nil) }} {
		log, err := parse(text)
		require.NoError(t, err)
		assert.Equal(t, want, log)
	}
}

// A run of tick lines in a hook log is one masked line, so a mock that ticks once or forty times matches a
// recording of four, and a mock that never ticks does not.
func TestHookLogMasksTheCountOfTicks(t *testing.T) {
	ticks := func(n int) string {
		return "{\"a\":1}\n" + strings.Repeat(tickLine+"\n", n) + "{\"a\":2}\n"
	}
	rec, err := parseHooks(ticks(4))
	require.NoError(t, err)
	for n, same := range map[int]bool{0: false, 1: true, 40: true} {
		mock, err := parseHooks(ticks(n))
		require.NoError(t, err)
		assert.Equal(t, same, assert.ObjectsAreEqual(rec, mock), "mock ticks %d", n)
	}
}
