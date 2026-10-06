package toolexec

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
		{"echo out; exit 3", true, "out"},
		{"exit 3", true, "Command failed with exit code 3"},
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
	if _, known := Required("Glob"); known {
		t.Error("Glob is not a tool the mock runs")
	}
	if c := FromScript("Grep", []byte(`{"pattern":"NEEDLE"}`)); c.Kind != "grepToolCall" || c.Name() != "Grep" || c.HookInput("/")["pattern"] != "NEEDLE" {
		t.Errorf("Grep: %+v", c)
	}
	if c := FromScript("mcp__local__echo", []byte(`{"text":"HI"}`)); c.Kind != "mcpToolCall" || c.Name() != "MCP:echo" || c.HookInput("/")["text"] != "HI" {
		t.Errorf("an MCP tool: %+v", c)
	}
	if req, known := Required("Read"); !known || len(req) != 1 || req[0] != "file_path" {
		t.Errorf("Read requires %v (known %v)", req, known)
	}
}

func TestAGrepWithAnyParameterButThePatternFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("NEEDLE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok := Execute(context.Background(), FromScript("Grep", []byte(`{"pattern":"NEEDLE"}`)), dir, nil)
	if ok.Failed {
		t.Fatalf("a Grep of a pattern alone failed: %s", ok.ErrorMessage)
	}
	r := Execute(context.Background(), FromScript("Grep", []byte(`{"pattern":"NEEDLE","path":"sub","-i":true}`)), dir, nil)
	if !r.Failed || !strings.Contains(r.ErrorMessage, "-i, path is not modeled") {
		t.Fatalf("a Grep with a path and -i: failed %v, %q", r.Failed, r.ErrorMessage)
	}
}

// sr:proves file-tools/cursor
func TestAStrReplaceEditsTheFileAndTheHooksSeeTheWholeFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := FromScript("Edit", []byte(`{"file_path":"note.txt","old_string":"hi","new_string":"bye"}`))
	if got := c.HookInput(dir)["content"]; got != "bye\n" {
		t.Fatalf("the hook's content = %q, want the whole file it makes", got)
	}
	r := Execute(context.Background(), c, dir, nil)
	if r.Failed {
		t.Fatal(r.ErrorMessage)
	}
	s := r.Frame["success"].(map[string]any)
	if s["diffString"] != "--- a/"+filepath.Join(dir, "note.txt")+"\n+++ b/"+filepath.Join(dir, "note.txt")+"\n@@ -1 +1 @@\n-hi\n+bye" || s["beforeFullFileContent"] != "hi\n" {
		t.Fatalf("result %+v", s)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "note.txt")); string(b) != "bye\n" {
		t.Fatalf("file %q", b)
	}
	// an old text that is not there once is not recorded: the call fails
	again := Execute(context.Background(), FromScript("Edit", []byte(`{"file_path":"note.txt","old_string":"zzz","new_string":"x"}`)), dir, nil)
	if !again.Failed {
		t.Fatal("an old text that is not in the file must fail the call")
	}
}

// A read reports content_length in characters the way the harness does (UTF-16
// units), not in bytes: a file with a non-ASCII character is shorter by the
// difference (recorded: runs/schedule-wakeup-ask).
func TestReadContentLengthCountsCharacters(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a—b\n"), 0o644); err != nil { // an em dash is 3 bytes, 1 character
		t.Fatal(err)
	}
	r := Execute(context.Background(), FromScript("Read", json.RawMessage(`{"file_path":"f.txt"}`)), dir, nil)
	var out struct {
		ContentLength int `json:"content_length"`
	}
	if err := json.Unmarshal([]byte(r.ToolOutput), &out); err != nil || out.ContentLength != 4 {
		t.Fatalf("content_length = %d (%v), want 4 characters", out.ContentLength, err)
	}
}

func TestACommandUsingTheHarnessRipgrepIsRefused(t *testing.T) {
	for _, cmd := range []string{`"$CURSOR_RIPGREP_PATH" foo .`, `~/.local/share/cursor-agent/versions/1/rg foo`} {
		r := Execute(context.Background(), Call{Kind: "shellToolCall", Args: map[string]any{"command": cmd}}, t.TempDir(), nil)
		if _, ok := r.NotModeled(); !ok {
			t.Errorf("%q was not refused: %+v", cmd, r)
		}
	}
	r := Execute(context.Background(), Call{Kind: "shellToolCall", Args: map[string]any{"command": "echo CURSOR_AGENT"}}, t.TempDir(), nil)
	if _, ok := r.NotModeled(); ok {
		t.Errorf("an ordinary command was refused")
	}
}
