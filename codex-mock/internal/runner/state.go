package runner

import (
	"io"

	"github.com/sloprail/harness-mocks/codex-mock/internal/events"
	"github.com/sloprail/harness-mocks/codex-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/codex-mock/internal/session"
	"github.com/sloprail/harness-mocks/internal/subagents"
	"github.com/sloprail/harness-mocks/internal/tasks"
	"github.com/sloprail/harness-mocks/internal/toolspec"
)

// Config is everything a run is told, read once at the entrypoint.
type Config struct {
	// Script is the scenario script that drives the agent.
	Script string
	Prompt string
	// Resume is the id of the session to continue, empty for a new one.
	Resume string
	// ForkFrom is the id of the session `exec fork` continues in a new one.
	ForkFrom string
	// Ephemeral is --ephemeral: nothing is kept, transcript_path is null (ephemeral.go).
	Ephemeral bool
	// Cwd is the session's working directory.
	Cwd string
	// CodexHome holds the user's hooks.json and the session's rollout.
	CodexHome string
	Model     string
	// Environ is the environment the mock was started with.
	Environ []string
	// JSON prints the event stream on Stdout; otherwise Stdout gets the agent's final message.
	JSON bool
	// BypassHookTrust is --dangerously-bypass-hook-trust: hooks run without review, with a warning.
	BypassHookTrust bool
	// DisableHooks is `--disable hooks`: no hook loads or runs.
	DisableHooks bool
	// IgnoreUserConfig is --ignore-user-config: config.toml is not read (hooks.Options).
	IgnoreUserConfig bool
	// Sandbox is the sandbox the run asked for (-s; danger-full-access for the bypass flag), empty for none.
	Sandbox        string
	Stdout, Stderr io.Writer
}

// state is the state of one run, shared by Codex's side of the turn and of
// its tool calls.
type state struct {
	cfg     Config
	id      string
	turnID  string
	hooks   *hooks.Invoker
	events  *events.Stream
	rollout *session.File
	toolEnv []string
	// bg holds the commands a call left running (see background.go).
	bg *tasks.Registry
	// prog is how far this agent is through its tool calls, parent how far the agent that
	// started it is (nil for the session's own), and spawned the sub-agents this one started
	// (see subagents.Hold: what a script's gate is read against).
	prog, parent *subagents.Progress
	// refused is the run's refusal of a script's call the mock does not implement (validate.go);
	// home is where the session's files go (ephemeral.go).
	refused  *toolspec.Refusals
	sessions *sessionLog // the commands a call left running, by session id (stdin_session.go)
	home     string
	spawned  *subagents.SpawnLog
}
