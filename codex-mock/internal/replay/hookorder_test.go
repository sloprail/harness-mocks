package replay

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func payload(event, tool string) map[string]any {
	return map[string]any{"hook_event_name": event, "turn_id": "t", "tool_use_id": tool, "session_id": "s"}
}

// Hooks of one event of one call log in any order; the events themselves, and
// two calls' payloads of the same event, keep theirs.
func TestOnlyConcurrentHooksAreSorted(t *testing.T) {
	objs := []map[string]any{
		payload("PreToolUse", "a"), payload("PreToolUse", "a"), {"ran": "x"},
		payload("PostToolUse", "a"),
		payload("PreToolUse", "b"),
	}
	assert.Equal(t, []int{0, 0, 0, 1, 2}, concurrentGroups(objs))
	assert.Equal(t, []string{"b", "c", "a", "y", "z"}, sortWithinGroups([]string{"c", "b", "a", "z", "y"}, []int{0, 0, 1, 2, 2}))
}
