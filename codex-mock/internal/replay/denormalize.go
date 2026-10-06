package replay

import (
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
	"github.com/sloprail/harness-mocks/internal/scenario"
)

// Scenario is what the mock is given to replay a recording: the run's own
// setup (its hooks.json, its hook script, its prompt) and the scenario script
// that makes the model's calls, in the format the mock takes of any scenario.
type Scenario struct {
	HooksJSON string
	// Files are written into the repository (the hook script).
	Files map[string]string
	// Scripts are the sub-agents' scenario scripts. The real run's repository
	// holds none, so they sit beside it, at scriptsDir, which the run's own
	// directory replaces.
	Scripts map[string]string
	Script  string
	Prompt  string
	// Flags are run options the mock is given (--ephemeral, -c agents.max_depth=N, -C dir).
	Flags []string
	// CmdFlags are the flags of the recorded command line (--json, --skip-git-repo-check ...); NoGit is a
	// run outside a repository; Exit the status the recorded run ended with.
	CmdFlags []string
	NoGit    bool
	Exit     string
	// Env is what the recorded run's process was given beyond the hermetic environment (setup/env).
	Env []string
	// Prepare is the recorded run's prepare.sh, run in the repository before the mock (prepare.go).
	Prepare string
	// ProjectHooksJSON is the project layer's hooks, written to <repo>/.codex/hooks.json.
	ProjectHooksJSON string
	// Interrupt is a run the user interrupted: the mock is sent SIGINT once its last command has started.
	Interrupt bool
	// Then are the later runs of the harness (a resume, a fork), each with its own script.
	Then []ThenStep
}

// ThenStep is a later run of the harness: its words, directory, prompt and script.
type ThenStep struct {
	Args   []string
	Cwd    string
	Prompt string
	Script string
}

// scriptsDir stands, in the scenario's calls, for the directory the sub-agents'
// scripts are written to.
const scriptsDir = "@SCRIPTS@"

// modelCall is one tool call the model made, in the mock's script vocabulary.
type modelCall struct {
	Text  *string        `json:"text,omitempty"` // what the model said just before the call, if it said anything
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
	// More is that another call of the model's script follows: it is not asked again after this call.
	More bool `json:"more,omitempty"`
	// Final is the end of a turn: the model's answer, with no call.
	Final *string `json:"-"`
	// Hang is an agent that never ends (an unfinished one).
	Hang bool `json:"-"`
	// Gate is what must have happened before the step is taken (gates.go).
	Gate scenario.Gate `json:"-"`
}

// runPlaceholder stands, in the scenario's calls, for the repository the mock runs
// in, which the replay's own directory replaces; the recording has it as <RUN>.
const runPlaceholder = "@RUN@"

// Denormalize turns the unified recording into the mock's scenario: the
// unified tools go back to the mock's names, and each sub-agent's turns become
// a script file of their own.
func Denormalize(rec core.Recording) Scenario {
	files := map[string]string{"hook.sh": rec.Setup["hook.sh"]}
	scripts := map[string]string{}
	main := agentCalls(rec.Agent, nil, scripts, new(int))
	return Scenario{
		HooksJSON:        rec.Setup["hooks.json"],
		Files:            files,
		Scripts:          scripts,
		Script:           scriptFor("main", 0, main.calls, rec.Agent.Final, false, main.final),
		Then:             thenScenario(rec),
		Prompt:           rec.Prompt,
		Flags:            strings.Fields(rec.Setup["flags"]),
		CmdFlags:         strings.Fields(rec.Setup["cmdflags"]),
		NoGit:            rec.Setup["no-git"] == "true",
		Exit:             rec.Setup["exit"],
		Env:              strings.Fields(rec.Setup["env"]),
		Prepare:          rec.Setup["prepare.sh"],
		ProjectHooksJSON: rec.Setup["project-hooks.json"],
		Interrupt:        rec.Agent.Interrupted,
	}
}
