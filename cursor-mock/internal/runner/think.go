package runner

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/scenario"
)

// Think tells the hooks what the model thought in a response (turnloop.Thinker):
// afterAgentThought fires once per response that has a thought, before its calls
// or, for the last, before the session ends, with the thinking text, how long the
// response took, the model that thought (the other keys of the script's thinking
// block, as the recording's payload names them) and the model call it is of
// (recorded: runs/task-stream-frames and the other recordings of the grok model;
// the newer default model says no thought at all).
func (s *session) Think(ctx context.Context, th scenario.Thought, took time.Duration) {
	own := map[string]any{"text": th.Text, "duration_ms": ms(took), "generation_id": s.modelCallID()}
	for k, raw := range th.Fields {
		var v any
		if json.Unmarshal(raw, &v) == nil {
			own[k] = v
		}
	}
	s.hooks.Fire(ctx, hooks.AfterAgentThought, "AgentThought", own) // a matcher is tested against AgentThought (recorded: runs/hook-matchers-thought)
}
