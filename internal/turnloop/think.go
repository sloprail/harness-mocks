package turnloop

import (
	"context"
	"time"

	"github.com/sloprail/harness-mocks/internal/scenario"
)

// Thinker is a Host that says what the model thought: a harness that tells its
// hooks of a response's thinking (Cursor's afterAgentThought). A host without it
// never says it, and a script's thinking blocks are read and left unsaid.
type Thinker interface {
	// Think is told of one thought of the response that is about to be said and
	// acted on, and how long the response took.
	Think(ctx context.Context, th scenario.Thought, took time.Duration)
}

// say is what the agent says of a turn: its thoughts first (before its messages
// and its calls, as the thinking precedes them), then its messages. It returns
// the last message, "" when there is none.
func say(ctx context.Context, h Host, t scenario.Turn, took time.Duration) (last string) {
	if th, ok := h.(Thinker); ok {
		for _, x := range t.Thoughts {
			th.Think(ctx, x, took)
		}
	}
	for _, text := range t.Texts {
		h.Say(text)
		last = text
	}
	return last
}
