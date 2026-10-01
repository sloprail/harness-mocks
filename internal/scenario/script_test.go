package scenario

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var environ = []string{"PATH=/usr/bin:/bin"}

func script(t *testing.T, body string) (path, dir string) {
	t.Helper()
	dir = t.TempDir()
	path = filepath.Join(dir, "s.sh")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, dir
}

// A tool_use line ends the turn: nothing after it is read. A result line ends
// the run.
// sr:proves turn-loop
func TestRunTurnEndsAtTheFirstToolUseOrResult(t *testing.T) {
	s, dir := script(t, `printf '%s\n' \
'{"type":"assistant","message":{"content":[{"type":"text","text":"one"},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}},{"type":"text","text":"never"}]}}' \
'{"type":"result","result":"never"}'`)
	turn, err := RunTurn(context.Background(), s, dir, environ, Input{})
	if err != nil {
		t.Fatal(err)
	}
	if len(turn.Texts) != 1 || turn.Texts[0] != "one" || turn.Result != nil {
		t.Fatalf("turn = %+v", turn)
	}
	if turn.Tool == nil || turn.Tool.ID != "t1" || turn.Tool.Name != "Bash" || string(turn.Tool.Input) != `{"command":"ls"}` {
		t.Fatalf("tool = %+v", turn.Tool)
	}

	s, dir = script(t, `printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}' '{"type":"result","result":"fin"}'`)
	turn, err = RunTurn(context.Background(), s, dir, environ, Input{})
	if err != nil || turn.Tool != nil || turn.Result == nil || *turn.Result != "fin" || turn.Texts[0] != "done" {
		t.Fatalf("turn = %+v, %v", turn, err)
	}
}

// The script is told the prompt unchanged, the session record's path, and the
// additional context a hook added, separately from the prompt.
// sr:proves scenario-prompt-env
// sr:proves session-file-env
// sr:proves prompt-context-appended
func TestRunTurnTellsTheScriptThePromptTheSessionFileAndTheContext(t *testing.T) {
	s, dir := script(t, `printf '{"type":"result","result":"%s|%s|%s"}\n' "$A10N_MOCK_PROMPT" "$A10N_MOCK_SESSION_FILE" "$A10N_MOCK_ADDITIONAL_CONTEXT"`)
	in := Input{Prompt: "fix the bug", AdditionalContext: "SS-CTX", SessionFile: "/x/rollout.jsonl"}
	turn, err := RunTurn(context.Background(), s, dir, append(environ, "A10N_MOCK_PROMPT=outer"), in)
	if err != nil || turn.Result == nil {
		t.Fatalf("turn = %+v, %v", turn, err)
	}
	if got, want := *turn.Result, "fix the bug|/x/rollout.jsonl|SS-CTX"; got != want {
		t.Fatalf("script saw %q, want %q", got, want)
	}
}

func TestRunTurnInvalidLineIsAnErrorNamingIt(t *testing.T) {
	s, dir := script(t, `echo 'not json'`)
	_, err := RunTurn(context.Background(), s, dir, environ, Input{})
	if err == nil || !strings.Contains(err.Error(), "not json") {
		t.Fatalf("err = %v, want one naming the line", err)
	}
}

func TestRunTurnScriptWithNeitherToolNorResultEnds(t *testing.T) {
	s, dir := script(t, `echo '{"type":"assistant","message":{"content":[{"type":"text","text":"just talk"}]}}'`)
	turn, err := RunTurn(context.Background(), s, dir, environ, Input{})
	if err != nil || turn.Tool != nil || turn.Result != nil || len(turn.Texts) != 1 {
		t.Fatalf("turn = %+v, %v", turn, err)
	}
}
