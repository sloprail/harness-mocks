package scenario

import (
	"context"
	"testing"
)

// A compact line asks for a compaction and ends the turn like a tool call
// does: nothing after it is read.
func TestRunTurnEndsAtACompactRequest(t *testing.T) {
	s, dir := script(t, `printf '%s\n' \
'{"type":"compact","trigger":"auto"}' \
'{"type":"result","result":"never"}'`)
	turn, err := RunTurn(context.Background(), s, dir, environ, Input{})
	if err != nil {
		t.Fatal(err)
	}
	if turn.Compact == nil || turn.Compact.Trigger != "auto" || turn.Result != nil || turn.Tool != nil {
		t.Fatalf("turn = %+v", turn)
	}
}
