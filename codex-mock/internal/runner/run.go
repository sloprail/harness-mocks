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

// Run starts the session, fires SessionStart, and runs one turn.
// sr:provides session-start-hook/codex
// sr:provides session-end-hook/codex
// sr:provides noninteractive-run/codex
func Run(ctx context.Context, cfg Config) error {
	if cfg.Script == "" {
		return errors.New("codex-mock: a scenario script is required (--script or A10N_MOCK_SCRIPT)")
	}
	if hooks.SandboxTrustsProject(cfg.Sandbox) { // Codex remembers the project it was asked to write in as trusted
		if err := hooks.PersistProjectTrust(cfg.CodexHome, cfg.Cwd); err != nil {
			return fmt.Errorf("codex-mock: cannot record the trusted project: %w", err)
		}
	}
	hookCfg, err := hooks.Load(cfg.CodexHome, cfg.Cwd, hooks.Options{Disabled: cfg.DisableHooks, BypassTrust: cfg.BypassHookTrust,
		IgnoreUserConfig: cfg.IgnoreUserConfig, Sandbox: cfg.Sandbox})
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
	s := &state{sessions: &sessionLog{}, home: home, refused: &toolspec.Refusals{}, prog: subagents.NewProgress(), spawned: &subagents.SpawnLog{}, cfg: cfg, id: id, turnID: coresession.NewID(), rollout: rollout, events: events.New(out),
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
	if !cfg.DisableHooks {
		for _, f := range hooks.AsyncSessionEndFiles(cfg.CodexHome, cfg.Cwd) {
			s.events.Warning("running async SessionEnd hook synchronously in " + f)
		}
		for _, w := range hooks.InterruptClampWarnings(cfg.CodexHome, cfg.Cwd) {
			s.events.Warning(w)
		}
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
	last, err := "", error(nil) // Codex honours a start hook's `continue: false` (runs/session-start-continue-false)
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
	s.hooks.Later.Wait() // the session ends with the run, after what ran in the background; what the hook prints is not read
	s.hooks.Fire(ctx, hooks.SessionEnd, "other", map[string]any{"reason": "other"})
	if !cfg.JSON && last != "" {
		fmt.Fprintln(cfg.Stdout, last)
	}
	return err
}
