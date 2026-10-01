// Package turn is the turn: the prompt hook, the agent's steps, the end-of-turn
// hook and the continuation a block of it asks for.
package turn

import (
	"context"
	"fmt"
	"strings"

	"github.com/sloprail/harness-mocks/codex-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/codex-mock/internal/scenario"
	"github.com/sloprail/harness-mocks/codex-mock/internal/toolcall"
)

// Deps is what a turn needs.
type Deps struct {
	toolcall.Deps
	Script  string
	Environ []string
	Prompt  string
}

// loopLimit is how many turns in a row a script may ask for the same call.
const loopLimit = 5

// Run is one turn. A prompt a hook blocks ends it at once, without the agent
// running; otherwise the agent runs until the script's result, the end-of-turn
// hooks then either let it end or block it, which continues the turn with the
// hook's reason as a new prompt.
//
// sr:docs https://developers.openai.com/codex/hooks#stop
func Run(ctx context.Context, d Deps) (string, error) {
	d.Events.TurnStarted()
	defer d.Events.TurnCompleted()
	extra, blocked := promptHooks(ctx, d)
	if blocked {
		return "", nil
	}
	d.Session.User(d.Prompt)
	continuing := false
	for {
		last, err := agent(ctx, d, extra)
		if err != nil {
			return "", err
		}
		reason, again := stopHooks(ctx, d, last, continuing)
		if !again {
			return last, nil
		}
		d.Session.User(fmt.Sprintf(`<hook_prompt hook_run_id="stop">%s</hook_prompt>`, reason))
		continuing = true
	}
}

// promptHooks fires UserPromptSubmit: the context the hooks add, and whether
// one blocked the prompt.
func promptHooks(ctx context.Context, d Deps) (extra string, blocked bool) {
	var texts []string
	own := map[string]any{"turn_id": d.TurnID, "prompt": d.Prompt}
	for _, o := range d.Hooks.Fire(ctx, hooks.UserPromptSubmit, "", own) {
		dec := hooks.Interpret(hooks.UserPromptSubmit, o)
		if dec.Blocked || dec.Denied {
			blocked = true
		}
		if dec.Context != "" {
			texts = append(texts, dec.Context)
			d.Session.Developer(dec.Context)
		}
	}
	return strings.Join(texts, "\n"), blocked
}

// stopHooks fires Stop: whether a hook blocked, and its reason.
func stopHooks(ctx context.Context, d Deps, last string, continuing bool) (string, bool) {
	own := map[string]any{"turn_id": d.TurnID, "stop_hook_active": continuing, "last_assistant_message": last}
	reason, again := "", false
	for _, o := range d.Hooks.Fire(ctx, hooks.Stop, "", own) {
		dec := hooks.Interpret(hooks.Stop, o)
		switch {
		case again:
		case dec.Blocked:
			reason, again = dec.BlockReason, true
		case dec.Denied:
			reason, again = dec.DenyReason, true
		}
	}
	return reason, again
}

// agent runs the script turn by turn until its result, returning its last
// message.
func agent(ctx context.Context, d Deps, extra string) (last string, err error) {
	var prev string
	same := 0
	for {
		t, err := scenario.RunTurn(ctx, d.Script, d.Dir, d.Environ, scenario.Input{
			Prompt: d.Prompt, AdditionalContext: extra, SessionFile: d.Session.Path})
		if err != nil {
			return "", err
		}
		for _, text := range t.Texts {
			d.Events.AgentMessage(text)
			d.Session.Assistant(text)
			last = text
		}
		if t.Tool == nil {
			return last, nil
		}
		// sr:invariant loop-guard
		key := t.Tool.Name + string(t.Tool.Input)
		if key == prev {
			same++
		} else {
			prev, same = key, 1
		}
		if same >= loopLimit {
			return "", fmt.Errorf("codex-mock: the scenario script emitted the same tool_use %d turns in a row", loopLimit)
		}
		toolcall.Run(ctx, d.Deps, *t.Tool)
	}
}
