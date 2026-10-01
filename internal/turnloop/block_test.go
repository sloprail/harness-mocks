package turnloop

import (
	"reflect"
	"testing"
)

func TestContinues(t *testing.T) {
	for _, tc := range []struct{ status, decision, want bool }{
		{false, false, false}, {true, false, true}, {false, true, true}, {true, true, true},
	} {
		if got := Continues(tc.status, tc.decision); got != tc.want {
			t.Errorf("Continues(%v, %v) = %v, want %v", tc.status, tc.decision, got, tc.want)
		}
	}
}

func TestAfterBlock(t *testing.T) {
	for _, tc := range []struct {
		blocks, cap int
		want        bool
	}{
		{1, 8, true}, {8, 8, true}, {9, 8, false}, {1, 1, true}, {2, 1, false},
		{1000, 0, true}, // no limit
	} {
		if got := AfterBlock(tc.blocks, tc.cap); got != tc.want {
			t.Errorf("AfterBlock(%d, %d) = %v, want %v", tc.blocks, tc.cap, got, tc.want)
		}
	}
}

func TestRunBlocksInARowEndAtTheCapAndTheNextIsOverridden(t *testing.T) {
	h := &host{cap: 2, stops: []string{"a", "b", "c", "d"}}
	last, err := run(t, h, 0, "")
	want := []string{"prompt", "say:done", "stop:done:false", "continue:a", "say:done", "stop:done:true",
		"continue:b", "say:done", "stop:done:true", "overridden:3"}
	if err != nil || last != "done" || !reflect.DeepEqual(h.log, want) {
		t.Fatalf("last=%q err=%v log=%v, want %v", last, err, h.log, want)
	}
}

func TestRunWithoutACapKeepsContinuing(t *testing.T) {
	h := &host{stops: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}}
	if _, err := run(t, h, 0, ""); err != nil {
		t.Fatal(err)
	}
	if h.nextStop != 10 {
		t.Fatalf("%d blocks honoured, want all 10", h.nextStop)
	}
}
