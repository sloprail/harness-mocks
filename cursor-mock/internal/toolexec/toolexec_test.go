package toolexec

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

var env = []string{"PATH=/usr/bin:/bin"}

func call(kind string, args map[string]any) Call { return Call{Kind: kind, Args: args} }

// sr:proves tool-failure-hook/cursor
func TestShellFailureAndErrorMessage(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		command string
		failed  bool
		message string
	}{
		{"echo fine", false, ""},
		{"false", true, "Command failed with exit code 1"},
		{"echo OOPS >&2; exit 3", true, "OOPS"},
		{"echo out; exit 3", true, "Command failed with exit code 3"},
	} {
		r := Execute(context.Background(), call("shellToolCall", map[string]any{"command": tc.command}), dir, env)
		if r.Failed != tc.failed || r.ErrorMessage != tc.message {
			t.Errorf("%q: failed %v message %q, want %v %q", tc.command, r.Failed, r.ErrorMessage, tc.failed, tc.message)
		}
	}
}

// sr:proves tool-failure-hook/cursor
func TestReadOfAMissingFileFailsAndOfAPresentOneSucceeds(t *testing.T) {
	dir := t.TempDir()
	r := Execute(context.Background(), call("readToolCall", map[string]any{"path": "a.txt"}), dir, env)
	if !r.Failed || r.ErrorMessage != "File not found: "+filepath.Join(dir, "a.txt") {
		t.Fatalf("missing file: %+v", r)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r = Execute(context.Background(), call("readToolCall", map[string]any{"path": "a.txt"}), dir, env); r.Failed {
		t.Fatalf("present file: %+v", r)
	}
}

func TestWriteReportsTheChangeWithoutTheTextTheContentsShare(t *testing.T) {
	dir := t.TempDir()
	w := func(content string) Result {
		return Execute(context.Background(), call("editToolCall", map[string]any{"path": "n.txt", "streamContent": content}), dir, env)
	}
	if r := w("hi\n"); len(r.Edits) != 1 || r.Edits[0] != (Edit{"", "hi\n"}) {
		t.Errorf("a new file: edits %+v, want the whole content as new_string", r.Edits)
	}
	if r := w("bye\n"); len(r.Edits) != 1 || r.Edits[0] != (Edit{"hi", "bye"}) {
		t.Errorf("a changed file: edits %+v, want hi -> bye", r.Edits)
	}
}

func TestAScriptsToolCallBecomesACursorCall(t *testing.T) {
	c := FromScript("Bash", []byte(`{"command":"ls","description":"x"}`))
	if c.Kind != "shellToolCall" || c.Command() != "ls" || c.Name() != "Shell" {
		t.Errorf("Bash: %+v", c)
	}
	c = FromScript("Write", []byte(`{"file_path":"/a/b","content":"hi"}`))
	if c.Kind != "editToolCall" || c.Path("/") != "/a/b" || c.Name() != "Write" || c.HookInput("/")["content"] != "hi" {
		t.Errorf("Write: %+v", c)
	}
	if _, known := Required("Grep"); known {
		t.Error("Grep is not a tool the mock runs")
	}
	if req, known := Required("Read"); !known || len(req) != 1 || req[0] != "file_path" {
		t.Errorf("Read requires %v (known %v)", req, known)
	}
}
