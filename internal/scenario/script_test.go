package scenario

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeScript(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.sh")
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// sr:proves scenario-prompt-env
// sr:proves session-file-env
// sr:proves prompt-context-appended
func TestScriptSeesPromptContextAndSessionFile(t *testing.T) {
	p := writeScript(t, "echo \"$A10N_MOCK_PROMPT|$A10N_MOCK_ADDITIONAL_CONTEXT|$A10N_MOCK_SESSION_FILE\"\n")
	lines, err := Script{Path: p, Environ: []string{"PATH=/usr/bin:/bin", "A10N_MOCK_PROMPT=outer"}}.Lines(context.Background(),
		Input{Prompt: "do it", AdditionalContext: "ctx", SessionFile: "/s/t.jsonl"})
	if err != nil || len(lines) != 1 {
		t.Fatalf("Lines = %q, %v", lines, err)
	}
	if got, want := string(lines[0]), "do it|ctx|/s/t.jsonl"; got != want {
		t.Fatalf("script saw %q, want %q: the prompt unchanged, the context apart from it", got, want)
	}
}

func TestLinesDropsEmptyLinesAndFailsOnExit(t *testing.T) {
	p := writeScript(t, "printf '{\"a\":1}\\n\\n  \\n{\"b\":2}\\n'\n")
	lines, err := Script{Path: p, Environ: []string{"PATH=/usr/bin:/bin"}}.Lines(context.Background(), Input{})
	if err != nil || len(lines) != 2 || !strings.HasPrefix(string(lines[1]), `{"b"`) {
		t.Fatalf("Lines = %q, %v", lines, err)
	}
	bad := writeScript(t, "exit 4\n")
	if _, err := (Script{Path: bad, Environ: []string{"PATH=/usr/bin:/bin"}}).Lines(context.Background(), Input{}); err == nil {
		t.Fatal("a script exiting non-zero must be an error")
	}
}
