package replay

import "github.com/sloprail/harness-mocks/internal/scenario"

// Scenario is what the mock is given to replay a recording: the run's own
// setup (its settings, its hook command, its prompt) and the scenario script
// that makes the model's calls, in the format the mock takes of any scenario.
type Scenario struct {
	Settings string
	Hook     string
	// Scripts are the sub-agents' scripts, by file name; the main one is Script.
	Scripts map[string]string
	Script  string
	Prompt  string
	// Earlier are the runs the setup's preparation makes before the run itself, each as its prompt
	// and the script file (by name, in Scripts) that makes its calls.
	Earlier []ScenarioEarlier
	// Then are the later runs of claude, each with its own script and the flags to run the mock with.
	Then []ScenarioStep
}

// ScenarioStep is a later run: its script, its prompt and the words given before it (a resume's
// <SESSION> is the first run's session).
type ScenarioStep struct {
	Script, Prompt string
	Args           []string
	// Cwd is the directory of the repository the run starts in (empty: its root), Symlink "<name> <target>"
	// a link made first, Settings and Hook the project files of that directory
	Cwd, Symlink, Settings, Hook string
}

// ScenarioEarlier is an earlier run: the prompt it is given (how the replay's claude knows it) and its script's file name.
type ScenarioEarlier struct{ Prompt, Script string }

// scriptCall is one tool call the model made, in the mock's script vocabulary.
type scriptCall struct {
	Text    *string        `json:"text,omitempty"` // what the model said just before the call, if it said anything
	Name    string         `json:"name"`
	Gated   bool           `json:"-"` // the receipt of its background command came after the command ended
	Silent  bool           `json:"-"` // the answer is a response with no visible output (only thinking)
	More    bool           `json:"-"` // another call of the same message follows this one
	Reply   string         `json:"-"` // an answer that ends a turn the harness goes on from (a Stop hook refuses to end it), not a call
	Early   []string       `json:"-"` // what the model said before Text
	Control bool           `json:"-"` // a control record of the mock (a compaction): Input is the record
	Input   map[string]any `json:"input"`
	// Gate is what must have happened before the step is taken (core.Gates).
	Gate scenario.Gate `json:"-"`
}
