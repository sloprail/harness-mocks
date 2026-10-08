package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/hooks"
	coresession "github.com/sloprail/harness-mocks/internal/session"
)

// compactionModel is the model the compaction's own call named in the recording (the session ran on
// Auto, which picks it): preCompact is the one payload of the session that says it.
const compactionModel = "gemini-3.7-flash-low"

// compress is /compress typed at the TUI's idle input (recorded: runs/tui-manual-compaction): a
// generation of its own, in which preCompact fires with trigger "manual" (it observes, and cannot
// block), then the compaction is told as a response with no text and a stop, neither carrying
// the usage of a model call. The transcript is not touched: no record of the command or of a
// summary is written, and what it held stays, byte for byte, in the same file (the real TUI
// appends only its own bookkeeping, which the replay does not compare). The session, the
// conversation and the transcript path stay what they were. How full the context was, and how many
// messages it held, are the real model's own counts, so they are fixed here.
//
// sr:provides manual-compaction/cursor
// sr:docs https://cursor.com/docs/hooks#precompact
func (s *session) compress(ctx context.Context) {
	s.gen = coresession.NewID()
	s.compactions++
	s.hooks.Fire(ctx, hooks.PreCompact, hooks.NoSubject, map[string]any{
		"model": compactionModel, "trigger": "manual", "context_usage_percent": 5.5, "context_tokens": 14000, "context_window_size": 256000,
		"message_count": 4, "messages_to_compact": 2, "is_first_compaction": s.compactions == 1,
	})
	s.hooks.Fire(ctx, hooks.AfterAgentResponse, "AgentResponse", map[string]any{"text": ""})
	for _, d := range s.hooks.Fire(ctx, hooks.Stop, "Stop", map[string]any{"status": "completed", "loop_count": 0}) {
		if d.Followup != "" {
			s.refusal.refuse("cursor-mock: a followup_message from the stop that ends a compaction is not modeled: it was not recorded")
		}
	}
}
