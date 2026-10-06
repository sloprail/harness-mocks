// Package runner is one `codex exec` run: the session, its hooks, and the turn.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/sloprail/harness-mocks/codex-mock/internal/childenv"
	"github.com/sloprail/harness-mocks/codex-mock/internal/events"
	"github.com/sloprail/harness-mocks/codex-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/codex-mock/internal/session"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
	coresession "github.com/sloprail/harness-mocks/internal/session"
	"github.com/sloprail/harness-mocks/internal/subagents"
	"github.com/sloprail/harness-mocks/internal/tasks"
	"github.com/sloprail/harness-mocks/internal/toolspec"
	"github.com/sloprail/harness-mocks/internal/turnloop"
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
	// Ephemeral is --ephemeral: nothing of the session is kept under CODEX_HOME, and its hooks'
	// payloads name no transcript (transcript_path null; recorded: runs/ephemeral-no-transcript).
	Ephemeral bool
	// Cwd is the session's working directory.
	Cwd string
	// CodexHome holds the user's hooks.json and the session's rollout.
	CodexHome string
	Model     string
	// Environ is the environment the mock was started with.
	Environ []string
	// JSON prints the event stream on Stdout; otherwise Stdout gets the
	// agent's final message.
	JSON bool
	// BypassHookTrust is --dangerously-bypass-hook-trust: hooks run without
	// review, and Codex warns of it.
	BypassHookTrust bool
	Stdout, Stderr  io.Writer
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
	// refused is the run's refusal of a script's call the mock does not implement (validate.go).
	refused *toolspec.Refusals
	spawned *subagents.SpawnLog
}

// Run starts the session, fires SessionStart, and runs one turn.
// sr:provides session-start-hook/codex
// sr:provides session-end-hook/codex
// sr:provides noninteractive-run/codex
func Run(ctx context.Context, cfg Config) error {
	if cfg.Script == "" {
		return errors.New("codex-mock: a scenario script is required (--script or A10N_MOCK_SCRIPT)")
	}
	hookCfg, err := hooks.Load(cfg.CodexHome, cfg.Cwd)
	if err != nil {
		return fmt.Errorf("codex-mock: cannot load hooks: %w", err)
	}
	home, cleanup, err := sessionHome(cfg)
	if err != nil {
		return err
	}
	defer cleanup()
	id, rollout, start, err := session.Start(home, cfg.Cwd, cfg.Resume, cfg.ForkFrom, time.Now())
	if err != nil {
		return fmt.Errorf("codex-mock: cannot open the session file: %w", err)
	}
	defer rollout.Close()
	out := cfg.Stdout
	if !cfg.JSON {
		out = io.Discard
	}
	s := &state{refused: &toolspec.Refusals{}, prog: subagents.NewProgress(), spawned: &subagents.SpawnLog{}, cfg: cfg, id: id, turnID: coresession.NewID(), rollout: rollout, events: events.New(out),
		toolEnv: childenv.ToolEnv(cfg.Environ, id), bg: tasks.NewRegistry()}
	defer s.bg.Shutdown()
	s.hooks = &hooks.Invoker{Config: hookCfg, Dir: cfg.Cwd, Environ: cfg.Environ, Ident: childenv.HookIdentity(),
		Common: hooks.Common{SessionID: id, TranscriptPath: transcriptOf(cfg, rollout), Cwd: cfg.Cwd, Model: cfg.Model,
			PermissionMode: "bypassPermissions"}, Later: &corehooks.Later{}, Step: s.prog.Started}
	if !cfg.JSON {
		s.events.Progress(cfg.Stderr, events.Header{Version: childenv.Version, Cwd: cfg.Cwd, Model: cfg.Model, Prompt: cfg.Prompt})
	}
	s.events.ThreadStarted(id)
	if cfg.BypassHookTrust {
		for range 2 { // as recorded: twice per run, with or without hooks (runs/noninteractive-run-no-git-check)
			s.events.Warning("`--dangerously-bypass-hook-trust` is enabled. Enabled hooks may run without review for this invocation.")
		}
	}
	for _, f := range hooks.AsyncSessionEndFiles(cfg.CodexHome, cfg.Cwd) {
		s.events.Warning("running async SessionEnd hook synchronously in " + f)
	}
	for _, w := range hooks.InterruptClampWarnings(cfg.CodexHome, cfg.Cwd) {
		s.events.Warning(w)
	}
	halted := false
	for _, o := range s.hooks.Fire(ctx, hooks.SessionStart, start.Source, map[string]any{"source": start.Source}) {
		d := hooks.Interpret(hooks.SessionStart, o)
		halted = halted || d.Halt
		if d.Context != "" {
			rollout.Developer(d.Context)
		}
	}
	s.events.TurnStarted()
	last, err := "", error(nil)
	// Codex honours a start hook's `continue: false` (runs/session-start-continue-false).
	if !corehooks.StartHookEndsTurn(halted, true) {
		turn, stop := signal.NotifyContext(ctx, os.Interrupt) // a user's Ctrl-C interrupts the turn, not the session
		last, err = turnloop.Run(turn, turnHost{s}, turnloop.Params{
			Script: cfg.Script, Dir: cfg.Cwd, Environ: cfg.Environ, Prompt: cfg.Prompt})
		if turn.Err() != nil && ctx.Err() == nil {
			err = s.interrupted(ctx)
		}
		stop()
	}
	s.events.TurnCompleted()
	s.reapAtExit()
	// The session ends with the run, for the one reason a non-interactive run has;
	// what the hook prints is not read.
	s.hooks.Later.Wait() // what ran in the background is over before the session ends
	s.hooks.Fire(ctx, hooks.SessionEnd, "other", map[string]any{"reason": "other"})
	if !cfg.JSON && last != "" {
		fmt.Fprintln(cfg.Stdout, last)
	}
	return err
}

// newID is a random identifier in the shape of a version 4 UUID.
