// Package replay compares what a mock produced with what a recording of the
// real harness shows. It is the harness-agnostic core: it speaks one unified
// format and holds no heuristic. A recording goes in as normalised turns (the
// model's calls, a tree of agents), and what is compared is normalised output
// (lines). Everything specific to a harness (how a recording is read into
// turns, how turns become the mock's script, which fields are not behaviour,
// in what order output is comparable) is an Adapter's.
package replay

import "time"

// The unified tool vocabulary of a recorded turn. An adapter maps its
// harness's tools onto these, and a tool it cannot map makes the recording
// unbuildable.
const (
	// ToolShell runs a command: Input "command" (string), "yield_time_ms" (int) and any other
	// option of the harness's own tool, as given (a string, number or boolean).
	ToolShell = "shell"
	// ToolPatch applies a patch to files: Input "patch" (string), in the harness's own patch format.
	ToolPatch = "apply_patch"
	// ToolSpawn starts a sub-agent: Input "message"; Call.Sub is its turns.
	ToolSpawn = "spawn_agent"
	// ToolPoll polls a command left running: "session" (its position among the commands the agent left
	// running), "yield_time_ms", "max_output_tokens".
	ToolPoll = "write_stdin"
	// ToolWait waits for sub-agents: "targets" ([]int, positions among the agent's spawns), "timeout_ms".
	ToolWait = "wait_agent"
	// ToolCompact is a compaction of the session: Input "trigger" ("auto" for one the harness made on its own
	// at a context limit, "manual" for /compact) and, when the recording gives them, "summary", "preserve",
	// "logical_parent", "preserved_segment" and "model_output".
	ToolCompact = "compact"
	// ToolAnswer is the end of a turn: the model's answer, with no call: Input "text" (string).
	// A turn that a hook continues is followed by more steps, so an agent can hold several.
	ToolAnswer = "answer"
	// ToolReadFile reads a file: Input "path" (string); other options of the harness's tool as given.
	ToolReadFile = "read_file"
	// ToolWriteFile writes a file whole: Input "path" and "content" (strings).
	ToolWriteFile = "write_file"
)

// Call is one tool call the model made.
type Call struct {
	Said *string // what the model said just before the call, if it said anything
	// SaidBefore is what it said earlier still, in order, when it said several things ahead of the call.
	SaidBefore []string
	Tool       string
	Input      map[string]any
	Sub        *Agent // the turns of the agent a ToolSpawn started, when they were recorded
	Ref        string // the harness's id of that agent (what its receipt named), when known
	More       bool   // another call of the same script follows: the model is not sampled between them
	// SameTurn: the model made this call in the same response as the previous one.
	SameTurn bool
	Thinking *Thinking      // what the model thought in the response this call begins, when recorded
	Compact  map[string]any // the compaction the harness made just before this response, as its hook said it, when it did
	// ExecEarly is that the call was carried out before the next step of the agents above it, as the
	// recording's stream shows: its gate does not wait for those steps.
	ExecEarly bool
	// At is when the harness made the call (an answer: gave it), and Done when the call's output
	// was given back, as recorded: how one agent's steps are ordered against another's.
	At, Done time.Time
}

// Agent is what one agent (the main one, or a sub-agent) did: its calls in
// order, then its final answer. A turn that a hook continues is followed by more
// steps, so an answer can sit among the calls (ToolAnswer): the final answer is
// the last one.
type Agent struct {
	Calls         []Call
	Final         string
	FinalThinking *Thinking // what the model thought before its final answer, when recorded
	ID            string    // the agent's own id, when the recording names it (a sub-agent's conversation)
	// Unfinished is an agent whose recording holds no final answer: it was still at work when
	// the run ended (a sub-agent the run did not wait for), and must not end in a replay either.
	Unfinished bool
	// FinalAt is when the final answer was given, as recorded.
	FinalAt time.Time
	// Interrupted is a run the user interrupted while the agent's last call ran (the call's output
	// says so): a replay interrupts it when that call has started, and the agent has no final answer.
	Interrupted bool
}

// Observed is what a run left that is compared, normalised: one line per
// event, in an order that is the behaviour.
type Observed struct {
	Events []string
	Hooks  []string
	// Checked is a replay that was a check the adapter made itself, passed, with nothing to compare (a
	// recording made with a flag the mock refuses replays as the check that the mock refuses it).
	Checked bool
	// NoStream says the harness's run has no event stream (a TUI draws a screen instead): its
	// hook payloads are what is compared, and an empty event stream is not a failure to compare.
	NoStream bool
	// Exits are how each step ended, one line per step ("exit 0"): a step the
	// harness refuses, with a status that is not 0, is behaviour too.
	Exits []string
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
	if want.Checked && got.Checked {
		return "", nil
	}
	if len(want.Events) == 0 && len(got.Events) == 0 && !(want.NoStream && got.NoStream && len(want.Hooks) > 0) {
		return "event stream: none recorded and none produced: nothing was compared\n", nil
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
