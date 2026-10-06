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
	assert.Equal(t, []int{0, 0, 0, 1, 2}, concurrentGroups(objs, nil))
	assert.Equal(t, []string{"b", "c", "a", "y", "z"}, sortWithinGroups([]string{"c", "b", "a", "z", "y"}, []int{0, 0, 1, 2, 2}))
}

// A Stop that a hook continued is followed by the Stop of the continued turn: they
// are in order, not at the same time; a leading line that is no payload opens the first group.
func TestAContinuedStopFollowsTheStopItContinues(t *testing.T) {
	stop := func(active bool) map[string]any {
		return map[string]any{"hook_event_name": "Stop", "turn_id": "t", "session_id": "s", "stop_hook_active": active}
	}
	objs := []map[string]any{{"ran": "first"}, stop(false), stop(false), stop(true), stop(true)}
	groups := concurrentGroups(objs, nil)
	assert.Equal(t, []int{0, 1, 1, 2, 2}, groups)
	lines := []string{"x", "b", "a", "d", "c"}
	assert.Equal(t, []string{"x", "a", "b", "c", "d"}, sortWithinGroups(lines, groups))
	assert.Equal(t, []string{"x", "b", "a", "d", "c"}, lines, "the input is left as it was")
}

// Hooks marked async log in whatever order they finish, whichever event started them: their
// events' lines, with nothing else between them, are one group; a synchronous event's are not.
func TestAsyncEventsHooksLogInOneGroup(t *testing.T) {
	objs := []map[string]any{
		{"hook_event_name": "SessionStart", "session_id": "s"},
		{"hook_event_name": "UserPromptSubmit", "turn_id": "t"},
		{"hook_event_name": "PostToolUse", "turn_id": "t", "tool_use_id": "u"},
		{"hook_event_name": "Stop", "turn_id": "t"},
	}
	async := map[string]bool{"SessionStart": true, "UserPromptSubmit": true}
	assert.Equal(t, []int{0, 0, 1, 2}, concurrentGroups(objs, async))
	assert.Equal(t, []int{0, 1, 2, 3}, concurrentGroups(objs, nil))
}
