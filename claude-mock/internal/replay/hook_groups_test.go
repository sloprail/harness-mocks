package replay

import (
	"reflect"
	"testing"
)

// The lines the handlers of one event log of their own sort among themselves;
// payload lines, in whatever order, stay where they are, so firings keep theirs.
func TestSortConcurrentSortsOnlyTheHandlersOwnLines(t *testing.T) {
	ev := func(name string) map[string]any { return map[string]any{"hook_event_name": name} }
	own := map[string]any{"hook_ran": "x"}
	objs := []map[string]any{ev("Stop"), own, own, own, ev("Stop"), ev("SessionEnd"), own}
	lines := []string{"stop-2", "c", "a", "b", "stop-1", "end", "z"}
	sortConcurrent(lines, objs)
	want := []string{"stop-2", "a", "b", "c", "stop-1", "end", "z"}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("got %v, want %v", lines, want)
	}
}
