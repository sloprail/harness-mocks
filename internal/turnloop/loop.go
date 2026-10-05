// Package turnloop is the harness-neutral turn: the prompt, the agent's steps
// driven by the scenario script, the end-of-turn hooks and the continuation a
// block of them asks for.
package turnloop

import (
	"context"
	"errors"
	"fmt"

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

// Params is what the loop is told.
type Params struct {
	Script  string
	Dir     string
	Environ []string
	Prompt  string
	// BlockCap is how many blocks in a row may continue the turn; zero is no
	// limit.
	BlockCap int
	// Added, when set, is the context the harness's hooks have added to the
	// conversation since the prompt (at the start of the session, after a tool
	// call), all of it so far in the order added. The script is given it as
	// additional context on each run, after what the prompt hooks added.
	Added func() string
}

// LoopLimit is how many turns in a row a script may ask for the same call.
const LoopLimit = 5

// Run is one turn and returns the agent's last message. A prompt a hook blocks
// ends it at once, the agent never running. Otherwise the script runs once per
// step: a tool call is carried out and the script runs again, until it gives
// its result; the end-of-turn hooks then let the turn end, or block it, which
// continues it with the reason as a new prompt. A script that asks for the
// same call LoopLimit steps in a row is stuck, and the run ends with an error.
func Run(ctx context.Context, h Host, p Params) (string, error) {
	extra, blocked := h.SubmitPrompt(ctx)
	if blocked {
		return "", nil
	}
	continuing := false
	blocks := 0
	for {
		last, err := agent(ctx, h, p, extra)
		if errors.Is(err, ErrTurnEnded) {
			return last, nil
		}
		if err != nil {
			return "", err
		}
		reason, blocked := h.EndOfTurn(ctx, last, continuing)
		if !Continues(blocked, false) {
			return last, nil
		}
		blocks++
		if !AfterBlock(blocks, p.BlockCap) {
			h.CapOverridden(blocks)
			return last, nil
		}
		h.Continue(reason)
		continuing = true
	}
}

// contextOf is the context the agent has at a step: what the prompt hooks added
// and, when the harness adds more as the turn goes, what it has added so far.
func contextOf(p Params, prompt string) string {
	if p.Added == nil || p.Added() == "" {
		return prompt
	}
	if prompt == "" {
		return p.Added()
	}
	return prompt + "\n" + p.Added()
}

// agent runs the script step by step until it gives its result (or neither a
// call nor a result), returning the agent's last message.
func agent(ctx context.Context, h Host, p Params, extra string) (last string, err error) {
	var prev string
	same := 0
	for {
		t, err := scenario.RunTurn(ctx, p.Script, p.Dir, p.Environ, scenario.Input{
			Prompt: p.Prompt, AdditionalContext: contextOf(p, extra), SessionFile: h.SessionFile()})
		if err != nil {
			return "", err
		}
		if g, ok := h.(Gater); ok && !t.Gate.None() {
			g.Gate(ctx, t.Gate)
		}
		for _, text := range t.Texts {
			h.Say(text)
			last = text
		}
		if t.Compact != nil {
			if c, ok := h.(Compactor); ok {
				if err := c.Compact(ctx, t.Compact.Trigger); err != nil {
					return last, err
				}
			}
		}
		if t.Tool == nil && t.Compact == nil {
			return last, nil
		}
		calls := turnCalls(h, t)
		// A scenario script that emits the same tool_use 5 turns in a row makes
		// the run abort with an error, so a stuck scenario cannot loop forever.
		// sr:invariant loop-guard
		key := "compact"
		if t.Compact != nil {
			key += " " + t.Compact.Trigger
		}
		if len(calls) > 0 {
			key = callsKey(calls)
		}
		if key == prev {
			same++
		} else {
			prev, same = key, 1
		}
		if same >= LoopLimit {
			return "", fmt.Errorf("the scenario script emitted the same tool_use %d turns in a row", LoopLimit)
		}
		perform(ctx, h, calls)
	}
}
