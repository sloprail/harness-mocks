package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/sloprail/harness-mocks/internal/tasks"
	"github.com/sloprail/harness-mocks/internal/turnloop"
)

// followUp is the prompt that starts the turn a finished sub-agent calls for
// (recorded: runs/print-waits-for-background-agents).
const followUp = "Perform any necessary follow-up actions in response to the subagent completion above. If no follow-up work is needed, no further action is required. If you mention an agent or subagent in your response, link it with the `[Name](id)` Don't use generic label such as `[agent]`, `[worker]`, or `[subagent]`. Don't repeat the same confirmation every time."

// runTurns plays the prompt's turn and then, while a background sub-agent is
// still running or has finished unreported, waits for it: its end is
// announced on the stream and starts a further turn, the script playing it
// with the follow-up prompt. The session ends, and its result is written, only
// when no sub-agent is left. The idle wait is unbounded: the mock's
// sub-agents are scripts, and a harness's own limit on it is not modeled.
//
// sr:provides print-waits-for-background-agents/cursor
func (s *session) runTurns(ctx context.Context) error {
	p := turnloop.Params{Script: s.cfg.Script, Dir: s.cfg.Dir, Environ: s.cfg.Environ, Prompt: s.cfg.Prompt, Added: s.Context}
	_, err := turnloop.Run(ctx, s, p)
	for err == nil {
		t := s.bg.AwaitAfterTurn(ctx, "")
		if t == nil {
			return nil
		}
		if t.Failure != "" {
			return fmt.Errorf("the sub-agent %q failed: %s", t.Description, t.Failure)
		}
		s.forward(notificationFrame(s.id, t))
		s.tr.followUp(followUp)
		p.Prompt = followUp
		_, err = turnloop.Run(ctx, s, p)
	}
	return err
}

// notificationFrame announces a sub-agent's end: its prompt, cut to 80
// characters, as the title and what it said as the detail.
func notificationFrame(session string, t *tasks.Task) []byte {
	prompt, _ := t.Meta.(string)
	title := []rune(prompt)
	if len(title) > 80 {
		title = title[:80]
	}
	return jsonLine(map[string]any{
		"type": "system", "subtype": "task_notification", "task_id": t.ID, "status": "success",
		"title": string(title), "detail": t.Result, "session_id": session, "timestamp_ms": time.Now().UnixMilli(),
	})
}

// followUp records the prompt of a further turn.
func (t *transcript) followUp(prompt string) {
	t.message("user", map[string]any{"type": "text", "text": "<timestamp/>\n\n<user_query>" + prompt + "</user_query>"})
}
