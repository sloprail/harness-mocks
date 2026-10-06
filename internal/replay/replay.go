// Package replay compares what a mock produced with what a recording of the
// real harness shows. It is the harness-agnostic core: it speaks one unified
// format and holds no heuristic. A recording goes in as normalised turns (the
// model's calls, a tree of agents), and what is compared is normalised output
// (lines). Everything specific to a harness (how a recording is read into
// turns, how turns become the mock's script, which fields are not behaviour,
// in what order output is comparable) is an Adapter's.
package replay

// The unified tool vocabulary of a recorded turn. An adapter maps its
// harness's tools onto these, and a tool it cannot map makes the recording
// unbuildable.
const (
	// ToolShell runs a command: Input "command" (string), optionally "yield_time_ms" (int); any other
	// option of the harness's own tool goes on as given (a string, number or boolean); what the mock does with it is the mock's.
	ToolShell = "shell"
	// ToolSpawn starts a sub-agent: Input "message" (string), and Call.Sub is the sub-agent's turns.
	ToolSpawn = "spawn_agent"
	// ToolAnswer is the end of a turn that the harness goes on from (a task's notification, a
	// hook's feedback): the model's answer, with no call: Input "text" (string). The agent's Final is
	// the last answer; the ones before it sit among the calls.
	ToolAnswer = "answer"
)

// Call is one tool call the model made.
type Call struct {
	Said *string // what the model said just before the call, if it said anything
	// SaidBefore is what it said earlier still, in order, when it said several things ahead of the call.
	SaidBefore []string
	Tool       string
	Input      map[string]any
	Sub        *Agent // the turns of the agent a ToolSpawn started, when they were recorded
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
	// Then are the later runs of the harness against the same session store (a resume, a fork),
	// in order, after the first.
	Then []Step
}

// Step is a later run of the harness against the store an earlier one left: what it was asked, the
// words it was given (a resume or a fork of the first session; opaque to the core), the directory it
// was run from (empty: the first run's), and what the model did in it.
type Step struct {
	Prompt string
	Args   []string
	Cwd    string
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
