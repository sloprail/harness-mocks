package compaction

import (
	"reflect"
	"strings"
	"testing"
)

func input(written []string) PlanInput {
	return PlanInput{
		WithSegment: true, Preserve: 2,
		Written:   func(n int) []string { return tail(written, n) },
		LastUUID:  written[len(written)-1],
		IsWritten: func(u string) bool { return contains(written, u) },
		NewUUID:   func() string { return "fresh" },
	}
}

func tail(xs []string, n int) []string {
	if len(xs) > n {
		xs = xs[len(xs)-n:]
	}
	return append([]string{}, xs...)
}

func TestPlanBoundary_KeepsTheLastRecordsAndContinuesFromTheTail(t *testing.T) {
	p := PlanBoundary(input([]string{"a", "b", "c", "d"}))
	if !reflect.DeepEqual(p.Kept, []string{"c", "d"}) || p.LogicalParent != "d" || !reflect.DeepEqual(p.All, []string{"c", "d"}) {
		t.Fatalf("plan = %+v", p)
	}
}

func TestPlanBoundary_TailOffsetEndsTheSegmentEarlier(t *testing.T) {
	in := input([]string{"a", "b", "c", "d", "e"})
	in.TailOffset = 2
	p := PlanBoundary(in)
	if !reflect.DeepEqual(p.Kept, []string{"b", "c"}) || p.LogicalParent != "c" {
		t.Fatalf("plan = %+v", p)
	}
}

func TestPlanBoundary_WithoutASegmentContinuesFromTheLastWrittenRecord(t *testing.T) {
	in := input([]string{"a", "b"})
	in.WithSegment = false
	p := PlanBoundary(in)
	if len(p.Kept) != 0 || p.LogicalParent != "b" || len(p.All) != 0 {
		t.Fatalf("plan = %+v", p)
	}
}

func TestPlanBoundary_UnwrittenParentClosesTheIDs(t *testing.T) {
	in := input([]string{"a", "b"})
	in.LogicalParent = Unwritten
	p := PlanBoundary(in)
	if p.LogicalParent != "fresh" || !reflect.DeepEqual(p.All, []string{"a", "b", "fresh"}) {
		t.Fatalf("plan = %+v", p)
	}
	in.LogicalParent = "given"
	if p := PlanBoundary(in); p.LogicalParent != "given" || p.All[len(p.All)-1] != "given" {
		t.Fatalf("plan = %+v", p)
	}
}

func steps(log *[]string, stop bool) Steps {
	mark := func(s string) func() { return func() { *log = append(*log, s) } }
	return Steps{
		Before:     func() bool { *log = append(*log, "before"); return stop },
		Summarizer: mark("summarizer"), Boundary: mark("boundary"), Summary: mark("summary"),
		Resume: mark("resume"), After: mark("after"), Command: mark("command"),
	}
}

func TestRun_ManualSequence(t *testing.T) {
	var log []string
	if !Run(true, steps(&log, false)) {
		t.Fatal("compaction did not happen")
	}
	if got := strings.Join(log, ","); got != "before,summarizer,boundary,summary,resume,after,command" {
		t.Fatalf("sequence = %s", got)
	}
}

func TestRun_AutomaticHasNoSummarizerOrCommand(t *testing.T) {
	var log []string
	Run(false, steps(&log, false))
	if got := strings.Join(log, ","); got != "before,boundary,summary,resume,after" {
		t.Fatalf("sequence = %s", got)
	}
}

func TestRun_TheHookBeforeCanStopIt(t *testing.T) {
	var log []string
	if Run(true, steps(&log, true)) {
		t.Fatal("a stopped compaction reports it happened")
	}
	if got := strings.Join(log, ","); got != "before" {
		t.Fatalf("sequence = %s, want nothing after the hook that stopped it", got)
	}
}
