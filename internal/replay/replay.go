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
	// ToolShell runs a command: Input "command" (string), optionally "yield_time_ms" (int); any other
	// option of the harness's own tool goes on as given (a string, number or boolean); what the mock does with it is the mock's.
	ToolShell = "shell"
	// ToolSpawn starts a sub-agent: Input "message" (string), and Call.Sub is the sub-agent's turns.
	ToolSpawn = "spawn_agent"
	// ToolAnswer is the end of a turn, the model's answer with no call: Input "text" (string).
	ToolAnswer = "answer"
	// ToolReadFile reads a file: Input "path" (string); other options of the harness's tool as given.
	ToolReadFile = "read_file"
	// ToolWriteFile writes a file whole: Input "path" and "content" (strings).
	ToolWriteFile = "write_file"
)

// Call is one tool call the model made.
type Call struct {
	Said  *string // what the model said just before the call, if it said anything
	Tool  string
	Input map[string]any
	Sub   *Agent // the turns of the agent a ToolSpawn started, when they were recorded
	// SameTurn: the model made this call in the same response as the previous one.
	SameTurn bool
	Thinking *Thinking // what the model thought in the response this call begins, when recorded
}

// Agent is what one agent (the main one, or a sub-agent) did: its calls in
// order, then its final answer; an answer a continued turn gave sits among the
// calls (ToolAnswer).
type Agent struct {
	Calls         []Call
	Final         string
	FinalThinking *Thinking // what the model thought before its final answer, when recorded
	ID            string    // the agent's own id, when the recording names it (a sub-agent's conversation)
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
	return Diff("event stream", want.Events, got.Events) + Diff("hook payloads", want.Hooks, got.Hooks) + Diff("exit statuses", want.Exits, got.Exits), nil
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
