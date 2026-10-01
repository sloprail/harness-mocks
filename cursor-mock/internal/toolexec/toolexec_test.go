package toolexec

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

var env = []string{"PATH=/usr/bin:/bin"}

// sr:proves tool-failure-hook/cursor
func TestShellOutcomeAndErrorMessage(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		command string
		outcome corehooks.ToolOutcome
		message string
	}{
		{"echo fine", corehooks.ToolSucceeded, ""},
		{"false", corehooks.ToolFailed, "Command failed with exit code 1"},
		{"echo OOPS >&2; exit 3", corehooks.ToolFailed, "OOPS"},
	} {
		r := Execute(context.Background(), Call{Kind: "shellToolCall", Args: map[string]any{"command": tc.command}}, dir, env)
		if r.Outcome != tc.outcome || r.ErrorMessage != tc.message {
			t.Errorf("%q: outcome %v message %q, want %v %q", tc.command, r.Outcome, r.ErrorMessage, tc.outcome, tc.message)
		}
	}
}

// sr:proves tool-failure-hook/cursor
func TestReadOfAMissingFileFailsAndOfAPresentOneSucceeds(t *testing.T) {
	dir := t.TempDir()
	r := Execute(context.Background(), Call{Kind: "readToolCall", Args: map[string]any{"path": "a.txt"}}, dir, env)
	if r.Outcome != corehooks.ToolFailed || r.ErrorMessage != "File not found: "+filepath.Join(dir, "a.txt") {
		t.Fatalf("missing file: %+v", r)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r = Execute(context.Background(), Call{Kind: "readToolCall", Args: map[string]any{"path": "a.txt"}}, dir, env)
	if r.Outcome != corehooks.ToolSucceeded {
		t.Fatalf("present file: %+v", r)
	}
}

func TestWriteReportsTheChangeWithoutTheTextTheContentsShare(t *testing.T) {
	dir := t.TempDir()
	w := func(content string) Result {
		return Execute(context.Background(), Call{Kind: "editToolCall", Args: map[string]any{"path": "n.txt", "streamContent": content}}, dir, env)
	}
	if r := w("hi\n"); len(r.Edits) != 1 || r.Edits[0] != (Edit{"", "hi\n"}) {
		t.Errorf("a new file: edits %+v, want the whole content as new_string", r.Edits)
	}
	if r := w("bye\n"); len(r.Edits) != 1 || r.Edits[0] != (Edit{"hi", "bye"}) {
		t.Errorf("a changed file: edits %+v, want hi -> bye", r.Edits)
	}
}

func TestAToolTheMockDoesNotRunFails(t *testing.T) {
	if r := Execute(context.Background(), Call{Kind: "grepToolCall"}, t.TempDir(), env); r.Outcome != corehooks.ToolErrored {
		t.Errorf("outcome %v, want ToolErrored", r.Outcome)
	}
}
