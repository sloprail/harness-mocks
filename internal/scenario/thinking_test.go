package scenario

import (
	"context"
	"testing"
)

// A thinking block is a thought of the turn: its text, and the other keys it holds
// for the harness; it ends nothing, and what follows it is read.
func TestRunTurnReadsAThinkingBlockBeforeTheCall(t *testing.T) {
	s, dir := script(t, `printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"hm","model":"grok","model_params":[1]},{"type":"text","text":"on it"},{"type":"tool_use","id":"t1","name":"Bash","input":{}}]}}'`)
	turn, err := RunTurn(context.Background(), s, dir, environ, Input{})
	if err != nil {
		t.Fatal(err)
	}
	if len(turn.Thoughts) != 1 || turn.Thoughts[0].Text != "hm" || string(turn.Thoughts[0].Fields["model"]) != `"grok"` || string(turn.Thoughts[0].Fields["model_params"]) != `[1]` {
		t.Fatalf("thoughts = %+v", turn.Thoughts)
	}
	if _, kept := turn.Thoughts[0].Fields["thinking"]; kept || len(turn.Texts) != 1 || turn.Tool == nil {
		t.Fatalf("turn = %+v", turn)
	}
}
