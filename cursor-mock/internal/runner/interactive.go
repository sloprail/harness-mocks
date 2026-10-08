package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/hooks"
	coresession "github.com/sloprail/harness-mocks/internal/session"
	"github.com/sloprail/harness-mocks/internal/turnloop"
)

// stopLoopLimit is how many follow-ups in a row a stop hook may ask for: Cursor's default
// loop_limit is 5 (https://cursor.com/docs/hooks#per-script-configuration-options).
const stopLoopLimit = 5

// turns is the state of an interactive session's turns.
type turns struct {
	ctx context.Context
	// prompt is the prompt of the turn the session is in.
	prompt string
	// gen names the turn the session is in: each prompt and each follow-up is a generation of its own.
	gen string
	// submitted: a prompt reached the agent (a refused one leaves no transcript).
	submitted bool
	// loops is how many follow-ups a stop hook has already triggered (the stop payload's loop_count).
	loops int
	// said is what the agent said since the generation began: the TUI tells it to the
	// afterAgentResponse hook once, when the generation ends.
	said []string
	// stopFollowUp: the turn goes on with the follow-up a stop hook gave.
	stopFollowUp bool
	// compactions is how many times /compress has compacted the conversation.
	compactions int
}

// interactive plays a TUI session: cursor-agent started without -p, whose typed lines (prompts, and
// /compress at the idle input) are the mock's stdin (the entrypoint reads it). It prints no
// stream, as a TUI draws a screen instead, and fires what only a TUI fires around its turns:
// beforeSubmitPrompt, afterAgentResponse and stop (recorded: runs/tui-stop, runs/tui-stop-followup,
// runs/tui-prompt-blocked, none of them fired in print mode). Each prompt is a turn of the same
// conversation, in the same transcript (recorded: runs/tui-multi-turn), and its tool calls are
// those of print mode (recorded: runs/tui-tools). The session's own hooks are as in print mode,
// except that no workspaceOpen is fired: whether the TUI fires one was not recorded.
//
// sr:docs https://cursor.com/docs/hooks#beforesubmitprompt
// sr:docs https://cursor.com/docs/hooks#afteragentresponse
func (s *session) interactive(ctx context.Context) error {
	s.ctx = ctx
	s.keep(s.hooks.Fire(ctx, hooks.SessionStart, hooks.NoSubject, map[string]any{"is_background_agent": false}))
	var runErr error
	for _, in := range s.cfg.Inputs {
		if strings.HasPrefix(in, "/") { // the entry point lets only /compress through
			if s.submitted {
				s.compress(ctx)
			}
			continue
		}
		s.prompt, s.loops = in, 0
		_, runErr = turnloop.Run(ctx, s, turnloop.Params{Script: s.cfg.Script, Dir: s.cfg.Dir, Environ: s.cfg.Environ,
			Prompt: in, BlockCap: stopLoopLimit, Added: s.Context})
		if runErr != nil || s.refusal.message() != "" {
			break
		}
	}
	if msg := s.refusal.message(); msg != "" {
		return errors.New(msg)
	}
	s.named = true
	s.hooks.Fire(ctx, hooks.SessionEnd, hooks.NoSubject, map[string]any{
		"reason": "completed", "duration_ms": time.Since(s.started).Milliseconds(),
		"is_background_agent": false, "final_status": "completed",
	})
	if s.submitted {
		s.tr.end()
	}
	if runErr != nil {
		return fmt.Errorf("cursor-mock: %w", runErr)
	}
	return nil
}

// submitPrompt fires beforeSubmitPrompt for the prompt the user sent: a hook that answers
// continue:false refuses it, and the agent never sees it. Otherwise it is the conversation's
// next message.
//
// sr:provides user-prompt-submit-hook/cursor
// sr:docs https://cursor.com/docs/hooks#beforesubmitprompt
func (s *session) submitPrompt(ctx context.Context) (string, bool) {
	s.gen = coresession.NewID()
	for _, d := range s.hooks.Fire(ctx, hooks.BeforeSubmitPrompt, "UserPromptSubmit", map[string]any{"prompt": s.prompt, "attachments": []any{}}) {
		if d.Refused {
			return "", true
		}
	}
	s.tr.user(s.prompt)
	s.submitted = true
	return "", false
}

// responded fires afterAgentResponse for what the agent said in the generation that ends, all of
// it as one message (recorded: runs/tui-tools, where the text said before a tool call and the
// answer after it are one text), which the transcript already holds: from here the hooks'
// payloads name the transcript.
func (s *session) responded() {
	text := strings.Join(s.said, "")
	s.said = nil
	s.tr.flushText()
	s.named = true
	s.hooks.Fire(s.ctx, hooks.AfterAgentResponse, "AgentResponse", s.withUsage(map[string]any{"text": text}))
}

// withUsage adds what the payloads of the end of a response carry of the model's usage: counts
// the real service measures, which the mock has no model to give, so they are fixed.
func (s *session) withUsage(p map[string]any) map[string]any {
	p["input_tokens"], p["output_tokens"], p["cache_read_tokens"], p["cache_write_tokens"] = 1000, 10, 500, 0
	return p
}
