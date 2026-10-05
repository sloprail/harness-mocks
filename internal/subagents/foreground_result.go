package subagents

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/internal/tasks"
)

// lineBreaks are what a hand-back normalises to "\n" before indenting: a
// carriage return, and the Unicode and control line separators.
var lineBreaks = regexp.MustCompile(`\r\n?|[\x{2028}\x{2029}\x{85}\x{0b}\x{0c}\x{1c}-\x{1e}]`)

// The foreground sub-agent result: what a parent is handed of a sub-agent it has
// waited for (HandBack), and the wait itself (Waits). One capability, marked once.

// HandBack is the text a foreground sub-agent hands its parent in place of its
// report: the harness's frame, then the report with every line indented, so a
// frame-like line inside it cannot be mistaken for the frame; empty stands in
// when the sub-agent said nothing. A foreground sub-agent blocks its parent
// until it has finished, and the parent gets this as the tool's result.
//
// sr:capability foreground-subagent-result
func HandBack(frame, empty, report string) string {
	if report == "" {
		report = empty
	}
	norm := lineBreaks.ReplaceAllString(report, "\n")
	return frame + "\n  " + strings.ReplaceAll(norm, "\n", "\n  ")
}

// Wait states of a sub-agent a wait tells of.
const (
	WaitCompleted = "completed"
	WaitRunning   = "running"
	WaitNotFound  = "not_found"
)

// WaitState is where one sub-agent a wait named stands.
type WaitState struct {
	ID     string
	Status string // WaitCompleted, WaitRunning or WaitNotFound
	Report string // a completed one's final report
}

// WaitResult is what a wait tells: each named sub-agent's state, in the order
// named (empty when the wait timed out), and whether it timed out.
type WaitResult struct {
	States   []WaitState
	TimedOut bool
}

// Timeout is a wait's timeout: ms when given, clamped into [lo, hi], else def (all milliseconds).
func Timeout(ms *int, def, lo, hi int) time.Duration {
	if ms == nil {
		return time.Duration(def) * time.Millisecond
	}
	return time.Duration(min(max(*ms, lo), hi)) * time.Millisecond
}

// Wait is the other half of the foreground sub-agent result, beside HandBack: it
// waits until one of the named sub-agents of the session's registry has
// finished, or timeout is up, and tells where each stands. Naming no sub-agent
// the registry holds, it returns at once.
func Wait(ctx context.Context, reg *tasks.Registry, ids []string, timeout time.Duration) (WaitResult, error) {
	ended := make(chan struct{}, len(ids))
	known := 0
	for _, id := range ids {
		if t := agentTask(reg, id); t != nil {
			known++
			go func() { <-t.Done(); ended <- struct{}{} }()
		}
	}
	if known > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-ended:
		case <-timer.C:
			return WaitResult{TimedOut: true}, nil
		case <-ctx.Done():
			return WaitResult{}, ctx.Err()
		}
	}
	var states []WaitState
	for _, id := range ids {
		switch t := agentTask(reg, id); {
		case t == nil:
			states = append(states, WaitState{ID: id, Status: WaitNotFound})
		case t.Finished():
			states = append(states, WaitState{ID: id, Status: WaitCompleted, Report: t.Result})
		default:
			states = append(states, WaitState{ID: id, Status: WaitRunning})
		}
	}
	return WaitResult{States: states}, nil
}

// agentTask is the sub-agent of the registry with this id; a background command
// of the same registry (its session id is a number) is no sub-agent to wait for.
func agentTask(reg *tasks.Registry, id string) *tasks.Task {
	if t := reg.Find(id); t != nil && t.Kind == tasks.Agent {
		return t
	}
	return nil
}
