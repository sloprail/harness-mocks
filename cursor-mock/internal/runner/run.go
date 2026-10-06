package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/childenv"
	"github.com/sloprail/harness-mocks/cursor-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/procexec"
	coresession "github.com/sloprail/harness-mocks/internal/session"
	"github.com/sloprail/harness-mocks/internal/tasks"
	"github.com/sloprail/harness-mocks/internal/turnloop"
)

// session is one run's state, and the harness side (turnloop.Host) of its turn.
type session struct {
	refusal *refusal // shared by the run's sessions: the first "not modeled" refusal ends the run
	cfg     Config
	id      string
	tr      *transcript
	hooks   *hooks.Hooks
	started time.Time
	// requestID names the run's model request: the result frame says it, and the
	// hooks of a Task call name it as their generation.
	requestID string
	texts     []string // what the agent said, in order: the result frame's text
	// pending is what the agent said that the stream has not shown yet: one frame
	// when the next call starts, or at the end of the turn (runs/foreground-subagent-failure).
	pending []string
	modelN  int // the model responses so far: a call's frames and the text before it name theirs
	// named: hook payloads carry the transcript path, null until the first tool
	// call is past its preToolUse hooks (recorded: runs/tool-failure).
	named bool
	// added is the context the hooks have handed the agent so far (their
	// additional_context), in the order their events fired and, within an event,
	// the order the hooks are configured in.
	added []string
	// owner is the id of the sub-agent conversation this session is, "" for the
	// main one, and parent the session that dispatched it: a sub-agent shares its
	// parent's background shells, owning what it starts.
	owner  string
	parent *session
	// owed: stream frames reporting what ended at a sub-agent's final response,
	// printed after the next tool call of this session, or at the end of its run.
	owed tasks.Deferred
}

// keep adds the context the hooks of one event gave to the agent's: all of it,
// when several hooks gave some.
//
// sr:provides hook-additional-context/cursor
func (s *session) keep(ds []hooks.Decision) {
	for _, d := range ds {
		if d.Context != "" {
			s.added = append(s.added, d.Context)
		}
	}
}

// Context is what the agent has been handed by hooks, all of it (turnloop.Params.Added).
func (s *session) Context() string { return strings.Join(s.added, "\n") }

// startHook is Cursor's sessionStart: it fires when a session begins, and a
// resumed one fires none (recorded: runs/session-resume).
//
// sr:provides session-resume/cursor
var startHook = coresession.StartPolicy{Fresh: coresession.StartHook{Fires: true}}

// Run plays one run: the stream's opening frames, the sessionStart hooks, the
// script's turns until its result, the sessionEnd hooks, and then the result
// frame, last on the stream. A single prompt runs to completion without
// interaction, streaming one record per line and ending with one result. The
// sessionEnd hook says the session completed: a non-interactive run has no
// other reason to report.
//
// The sessionStart hook fires once, before the first turn, and what it prints or
// exits with does not stop the session (recorded: runs/session-start-block); the
// payload says no source, as Cursor's does not.
//
// sr:provides noninteractive-run/cursor
// sr:provides session-end-hook/cursor
// sr:provides session-start-hook/cursor
// sr:docs https://cursor.com/docs/hooks#sessionend
// sr:docs https://cursor.com/docs/hooks#sessionstart
func Run(ctx context.Context, cfg Config) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s := &session{cfg: cfg, id: cfg.Resume, started: time.Now(), requestID: coresession.NewID(), refusal: &refusal{cancel: cancel}}
	first := s.requestID // the result frame names the run's first request, whatever turns follow
	if s.id == "" {
		s.id = coresession.NewID()
	}
	var err error
	if s.tr, err = newTranscript(cfg.Home, cfg.Dir, s.id); err != nil {
		return fmt.Errorf("cursor-mock: %w", err)
	}
	conf, err := hooks.Load(cfg.Dir, cfg.Home, cfg.PluginDirs...)
	if err != nil {
		return fmt.Errorf("cursor-mock: %w", err)
	}
	s.hooks = &hooks.Hooks{Config: conf, Dir: cfg.Dir, Env: s.hookEnv, Common: s.common}
	if cfg.Resume != "" { // the workspace holds the session's transcript only if it was begun there
		_ = coresession.ContinueTranscript(s.tr.path, func(l string) bool { return strings.Contains(l, `"turn_ended"`) })
	}
	s.forward(initFrame(s.id, cfg.Dir, cfg.Model))
	s.forward(userFrame(s.id, cfg.Prompt))
	s.hooks.Fire(ctx, hooks.WorkspaceOpen, hooks.NoSubject, nil) // the app opens the workspace before the session starts (recorded: runs/workspace-open)
	if startHook.For(cfg.Resume != "").Fires {
		s.keep(s.hooks.Fire(ctx, hooks.SessionStart, hooks.NoSubject, map[string]any{"is_background_agent": false}))
	}
	s.tr.user(cfg.Prompt) // the transcript file does not exist yet when the start hook runs
	_, runErr := turnloop.Run(ctx, s, turnloop.Params{Script: cfg.Script, Dir: cfg.Dir, Environ: cfg.Environ, Prompt: cfg.Prompt, Added: s.Context})
	if msg := s.refusal.message(); msg != "" { // a refusal of something not modeled fails the run, wherever it was made
		return errors.New(msg)
	}
	s.named = true
	s.hooks.Fire(ctx, hooks.SessionEnd, hooks.NoSubject, map[string]any{
		"reason": "completed", "duration_ms": time.Since(s.started).Milliseconds(),
		"is_background_agent": false, "final_status": "completed",
	})
	s.tr.end()
	if runErr != nil {
		return fmt.Errorf("cursor-mock: %w", runErr)
	}
	s.flushOwed()
	s.flushText(false)
	s.forward(resultFrame(s.id, first, strings.Join(s.texts, ""), time.Since(s.started)))
	return nil
}

// hookEnv is the environment of a hook command: the harness's own, with the
// facts Cursor gives a hook (recorded: runs/subprocess-session-env,
// runs/nested-session-env).
func (s *session) hookEnv() []string {
	return procexec.Env(s.cfg.Environ,
		childenv.HookIdentity(s.cfg.Dir, s.common().TranscriptPath, s.cfg.Version), childenv.HookDefaults(s.cfg.Dir, s.cfg.Version))
}

// common is what every hook payload carries now: the transcript path only once
// the conversation has a transcript.
func (s *session) common() hooks.Common {
	c := hooks.Common{SessionID: s.id, Dir: s.cfg.Dir, Version: s.cfg.Version, Model: s.cfg.Model}
	if s.named && s.tr.exists() {
		c.TranscriptPath = s.tr.path
	}
	return c
}
