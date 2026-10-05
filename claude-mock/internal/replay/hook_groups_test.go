package replay

import (
	"reflect"
	"testing"
)

// The handlers of one firing sort among themselves; the firings keep their order.
func TestSortConcurrentKeepsTheOrderOfFirings(t *testing.T) {
	objs := []map[string]any{
		{"hook_event_name": "Stop", "stop_hook_active": false}, {"hook_event_name": "Stop", "stop_hook_active": false},
		{"hook_event_name": "Stop", "stop_hook_active": true},
		{"hook_event_name": "SessionEnd"},
	}
	lines := []string{"b", "a", "c", "d"}
	sortConcurrent(lines, objs)
	if want := []string{"a", "b", "c", "d"}; !reflect.DeepEqual(lines, want) {
		t.Fatalf("got %v, want %v", lines, want)
	}
	// a later firing's line never moves before an earlier one's
	later := []string{"z", "a"}
	sortConcurrent(later, []map[string]any{{"hook_event_name": "PreToolUse", "tool_use_id": "1"}, {"hook_event_name": "PostToolUse", "tool_use_id": "1"}})
	if !reflect.DeepEqual(later, []string{"z", "a"}) {
		t.Fatalf("firings reordered: %v", later)
	}
}
