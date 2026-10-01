// Package runner is one `codex exec` run: the session, its hooks, and the turn.
package runner

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/sloprail/harness-mocks/codex-mock/internal/childenv"
	"github.com/sloprail/harness-mocks/codex-mock/internal/events"
	"github.com/sloprail/harness-mocks/codex-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/codex-mock/internal/session"
	"github.com/sloprail/harness-mocks/codex-mock/internal/toolcall"
	"github.com/sloprail/harness-mocks/codex-mock/internal/turn"
)

// Config is everything a run is told, read once at the entrypoint.
type Config struct {
	// Script is the scenario script that drives the agent.
	Script string
	Prompt string
	// Cwd is the session's working directory.
	Cwd string
	// CodexHome holds the user's hooks.json and the session's rollout.
	CodexHome string
	Model     string
	// Environ is the environment the mock was started with.
	Environ []string
	// JSON prints the event stream on Stdout; otherwise Stdout gets the
	// agent's final message.
	JSON           bool
	Stdout, Stderr io.Writer
}

// Run starts the session, fires SessionStart, and runs one turn.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Script == "" {
		return errors.New("codex-mock: a scenario script is required (--script or A10N_MOCK_SCRIPT)")
	}
	hookCfg, err := hooks.Load(cfg.CodexHome, cfg.Cwd)
	if err != nil {
		return fmt.Errorf("codex-mock: cannot load hooks: %w", err)
	}
	id := newID()
	rollout, err := session.Create(cfg.CodexHome, id, cfg.Cwd, time.Now())
	if err != nil {
		return fmt.Errorf("codex-mock: cannot create the session file: %w", err)
	}
	defer rollout.Close()
	out := cfg.Stdout
	if !cfg.JSON {
		out = io.Discard
	}
	stream := events.New(out)
	stream.ThreadStarted(id)
	iv := &hooks.Invoker{Config: hookCfg, Dir: cfg.Cwd, Environ: cfg.Environ, Ident: childenv.HookIdentity(),
		Common: hooks.Common{SessionID: id, TranscriptPath: rollout.Path, Cwd: cfg.Cwd, Model: cfg.Model,
			PermissionMode: "bypassPermissions"}}
	for _, o := range iv.Fire(ctx, hooks.SessionStart, "startup", map[string]any{"source": "startup"}) {
		if dec := hooks.Interpret(hooks.SessionStart, o); dec.Context != "" {
			rollout.Developer(dec.Context)
		}
	}
	last, err := turn.Run(ctx, turn.Deps{
		Deps: toolcall.Deps{Hooks: iv, Env: childenv.ToolEnv(cfg.Environ, id), Dir: cfg.Cwd, Events: stream,
			Session: rollout, Stderr: cfg.Stderr, TurnID: newID()},
		Script: cfg.Script, Environ: cfg.Environ, Prompt: cfg.Prompt,
	})
	if !cfg.JSON && last != "" {
		fmt.Fprintln(cfg.Stdout, last)
	}
	return err
}

// newID is a random identifier in the shape of a version 4 UUID.
func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
