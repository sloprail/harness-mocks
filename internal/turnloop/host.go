package turnloop

import (
	"context"

	"github.com/sloprail/harness-mocks/internal/scenario"
)

// Host is the harness side of a turn: its hooks and its record of the session.
type Host interface {
	// SubmitPrompt fires the prompt hooks: the context they add, and whether
	// one blocked the prompt.
	SubmitPrompt(ctx context.Context) (extra string, blocked bool)
	// Say records a message from the agent.
	Say(text string)
	// Tool carries out the tool call the agent made.
	Tool(ctx context.Context, tu scenario.ToolUse)
	// EndOfTurn fires the end-of-turn hooks after the agent's last message,
	// continuing being true when an earlier block already continued the turn;
	// again is whether one blocked, with the reason it gave.
	EndOfTurn(ctx context.Context, last string, continuing bool) (reason string, again bool)
	// Continue records the reason a block gave, as the prompt that continues
	// the turn.
	Continue(reason string)
	// CapOverridden records that a block was overridden: blocks in a row had
	// already continued the turn as often as the cap allows, so it ends.
	CapOverridden(blocks int)
	// SessionFile is the path of the session record so far.
	SessionFile() string
}

// Gater is a host that holds a turn back until what its script's gate names has
// happened (scenario.Gate): the script, not the time anything takes, orders the
// agents' steps.
type Gater interface {
	Gate(ctx context.Context, g scenario.Gate)
}

// Noticer is a host whose agent is told, as its work goes on, of what ended meanwhile
// (a sub-agent it started). When the agent is told is the loop's: after the last call
// of a script of the model (not between the calls one script made, see
// scenario.ToolUse.More), and when the agent's turn would end, which then goes on
// with it instead of ending, no end-of-turn hook having run and no block counted.
type Noticer interface {
	// Notice is what the agent is now to be told of, in order: after the last call of a script of
	// the model, or, with endOfTurn, as its turn would end.
	Notice(endOfTurn bool) []string
	// Told records that the agent was told it.
	Told(text string)
}
