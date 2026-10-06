package runner

import (
	"context"
	"encoding/json"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/scenario"
)

// CompactWith is a compaction the script asks for (turnloop.FieldCompactor): the
// preCompact hooks are told how full the context was, in the numbers the script's compact
// line gives (the real harness counts tokens with its model's tokenizer, which the
// mock has none of), and the transcript has the prompt written into it again, after
// the records the compaction dropped: the only record a compaction leaves in it
// (recorded: runs/compaction-transcript-continuity, where the prompt is
// byte-identical to the first record). The hook's generation is the model request.
//
// sr:docs https://cursor.com/docs/hooks#precompact
func (s *session) CompactWith(ctx context.Context, c scenario.Compact) error {
	own := map[string]any{"trigger": c.Trigger, "generation_id": s.requestID}
	for k, raw := range c.Fields {
		var v any
		if json.Unmarshal(raw, &v) == nil {
			own[k] = v
		}
	}
	s.hooks.Fire(ctx, hooks.PreCompact, hooks.NoSubject, own)
	s.tr.user(s.cfg.Prompt)
	return nil
}
