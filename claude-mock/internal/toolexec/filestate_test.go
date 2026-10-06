package toolexec

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func call(t *testing.T, ctx context.Context, dir, tool, input string) Result {
	t.Helper()
	return Execute(ctx, tool, json.RawMessage(input), dir, "sess")
}

// An Edit says the file is current in the agent's context when the agent wrote it or read all of it, and
// not after reading only part of it; the structured result flags the content as outside the model's
// context exactly when it does not say so; another agent's context is its own (recorded: runs/file-tools,
// fgsub-tool-stats).
func TestEditSaysTheFileIsCurrentWhenTheAgentHoldsIt(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.txt")
	main, other := context.Background(), WithAgent(context.Background(), "agent-1")
	edit := func(ctx context.Context, from, to string) Result {
		return call(t, ctx, dir, "Edit", `{"file_path":"`+f+`","old_string":"`+from+`","new_string":"`+to+`"}`)
	}
	current := func(r Result) bool {
		_, flagged := r.ToolUseResult.(map[string]any)["contentNotInModelContext"]
		if strings.HasSuffix(r.Output, "no need to Read it back)") == flagged {
			t.Fatalf("the note and the flag disagree: %q %v", r.Output, r.ToolUseResult)
		}
		return !flagged
	}
	call(t, main, dir, "Write", `{"file_path":"`+f+`","content":"a\nb\nc\n"}`)
	if !current(edit(main, "a", "A")) {
		t.Fatal("an Edit after the agent's own Write")
	}
	if current(edit(other, "b", "B")) {
		t.Fatal("another agent never held the file")
	}
	call(t, main, dir, "Read", `{"file_path":"`+f+`","offset":2,"limit":1}`)
	if current(edit(main, "c", "C")) {
		t.Fatal("after a read of part of it")
	}
	call(t, main, dir, "Read", `{"file_path":"`+f+`"}`)
	if !current(edit(main, "B", "b")) {
		t.Fatal("after a read of all of it")
	}
	if b, _ := os.ReadFile(f); string(b) != "A\nb\nC\n" {
		t.Fatalf("file %q", b)
	}
}
