// Package replay compares what a mock produced with what a recording of the
// real harness shows. It is the harness-agnostic core: it speaks one unified
// format and holds no heuristic. A recording goes in as normalised turns (the
// model's calls, a tree of agents), and what is compared is normalised output
// (lines). Everything specific to a harness (how a recording is read into
// turns, how turns become the mock's script, which fields are not behaviour,
// in what order output is comparable) is an Adapter's.
package replay

import (
	"fmt"
	"strings"
)

// The unified tool vocabulary of a recorded turn. An adapter maps its
// harness's tools onto these, and a tool it cannot map makes the recording
// unbuildable.
const (
	// ToolShell runs a command: Input "command" (string), "yield_time_ms" (int) and any other
	// option of the harness's own tool, as given (a string, number or boolean).
	ToolShell = "shell"
	// ToolSpawn starts a sub-agent: Input "message"; Call.Sub is its turns.
	ToolSpawn = "spawn_agent"
	// ToolWait waits for sub-agents: Input "targets" ([]int, each the position of its
	// ToolSpawn among the agent's spawns) and "timeout_ms" (int).
	ToolWait = "wait_agent"
)

// Call is one tool call the model made.
type Call struct {
	Said  *string // what the model said just before the call, if it said anything
	Tool  string
	Input map[string]any
	Sub   *Agent // the turns of the agent a ToolSpawn started, when they were recorded
}

// Agent is what one agent (the main one, or a sub-agent) did: its calls in
// order, then its final answer.
type Agent struct {
	Calls []Call
	Final string
}

// Recording is a recorded run in unified form.
type Recording struct {
	Dir    string            // the recorded run: an adapter reads what it needs of it here
	Prompt string            // what the agent was asked
	Setup  map[string]string // the run's own setup, by name: opaque to the core
	Agent  Agent
}

// Observed is what a run left that is compared, normalised: one line per
// event, in an order that is the behaviour.
type Observed struct {
	Events []string
	Hooks  []string
}

// Adapter is everything a harness contributes to a replay.
type Adapter interface {
	// Load reads the recorded run into unified form; an *Unbuildable says what the adapter cannot reproduce.
	Load(runDir string) (Recording, error)
	// Script is the mock's scenario for rec, printed for debugging.
	Script(rec Recording) (string, error)
	// Replay runs the mock on rec and returns the recording's output and the
	// mock's, each normalised the same way. A mock that fails is a *MockFailure.
	Replay(mock string, rec Recording) (want, got Observed, err error)
}

// Unbuildable says what of a recording an adapter cannot reproduce yet.
type Unbuildable struct{ Reason string }

func (u *Unbuildable) Error() string { return u.Reason }

// MockFailure is a mock that did not run to the end.
type MockFailure struct{ Detail string }

func (m *MockFailure) Error() string { return "the mock failed: " + m.Detail }

// Run replays the recording in runDir through a and returns what differs;
// empty is a green replay.
func Run(a Adapter, mock, runDir string) (string, error) {
	defer hold()()
	rec, err := a.Load(runDir)
	if err != nil {
		return "", err
	}
	want, got, err := a.Replay(mock, rec)
	if f, ok := err.(*MockFailure); ok {
		return f.Error(), nil
	}
	if err != nil {
		return "", err
	}
	return Diff("event stream", want.Events, got.Events) + Diff("hook payloads", want.Hooks, got.Hooks), nil
}

// Script is the scenario a generates for the recording in runDir.
func Script(a Adapter, runDir string) (string, error) {
	rec, err := a.Load(runDir)
	if err != nil {
		return "", err
	}
	return a.Script(rec)
}

// Diff is empty when want and got are the same lines; otherwise it shows the
// first lines that differ and the counts.
func Diff(what string, want, got []string) string {
	if len(want) == len(got) {
		same := true
		for i := range want {
			if want[i] != got[i] {
				same = false
				break
			}
		}
		if same {
			return ""
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: recording has %d lines, mock has %d\n", what, len(want), len(got))
	shown := 0
	for i := 0; i < len(want) || i < len(got) && shown < 3; i++ {
		var w, g string
		if i < len(want) {
			w = want[i]
		}
		if i < len(got) {
			g = got[i]
		}
		if w != g {
			fmt.Fprintf(&b, "line %d differs\n  recording: %s\n  mock:      %s\n", i+1, clip(w), clip(g))
			if shown++; shown == 3 {
				break
			}
		}
	}
	return b.String()
}

func clip(s string) string {
	if len(s) > 600 {
		return s[:600] + "…"
	}
	if s == "" {
		return "(none)"
	}
	return s
}
