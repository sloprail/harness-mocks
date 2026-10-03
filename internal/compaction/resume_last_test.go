package compaction

import (
	"strings"
	"testing"
)

func TestDo_AStopAfterTheCompactionLeavesItMadeAndSkipsTheSessionStart(t *testing.T) {
	var log []string
	s := steps(&log, false)
	s.ResumeLast = true
	s.AfterStops = func() bool { log = append(log, "after-stops"); return true }
	res := Do(false, s)
	if !res.Happened || !res.Stopped {
		t.Fatalf("result = %+v", res)
	}
	if got := strings.Join(log, ","); got != "before,boundary,summary,after-stops" {
		t.Fatalf("sequence = %s", got)
	}
}

func TestRun_ResumeLastFiresTheSessionStartAfterTheHookAfter(t *testing.T) {
	var log []string
	s := steps(&log, false)
	s.ResumeLast = true
	Run(false, s)
	if got := strings.Join(log, ","); got != "before,boundary,summary,after,resume" {
		t.Fatalf("sequence = %s", got)
	}
}
