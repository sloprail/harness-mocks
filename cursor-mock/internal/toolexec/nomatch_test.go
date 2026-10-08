package toolexec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runShell(t *testing.T, dir, cmd string) map[string]any {
	t.Helper()
	r := Execute(context.Background(), Call{Kind: "shellToolCall", Args: map[string]any{"command": cmd}}, dir, os.Environ())
	for _, k := range []string{"success", "failure"} {
		if b, ok := r.Frame[k].(map[string]any); ok {
			return b
		}
	}
	t.Fatalf("no result in %+v", r)
	return nil
}

// Cursor's shell is zsh (recorded: runs/shell-glob-tilde): a glob that matches expands, one that
// matches nothing fails its command with status 1 and zsh's message (the command does not run,
// the rest of the line carries on), and a quoted glob and `a~b` stay as they are.
func TestAGlobMatchingNothingFailsItsCommandAsZshDoes(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		cmd, stdout, stderr string
		code                int
	}{
		{`echo *.txt`, "a.txt b.txt\n", "", 0},
		{`echo [ab].txt ?.txt`, "a.txt b.txt a.txt b.txt\n", "", 0},
		{`echo "*.txt" a~b`, "*.txt a~b\n", "", 0},
		{`echo *.nomatch`, "", "(eval):1: no matches found: *.nomatch\n", 1},
		{`echo *.txt *.nomatch`, "", "(eval):1: no matches found: *.nomatch\n", 1},
		{`echo x[1]y`, "", "(eval):1: no matches found: x[1]y\n", 1},
		{`echo -c a=+r/*:r/*`, "", "(eval):1: no matches found: a=+r/*:r/*\n", 1},
		{`echo before; echo *.nomatch || echo fallback`, "before\nfallback\n", "(eval):1: no matches found: *.nomatch\n", 0},
		{`[ -f a.txt ] && echo ok`, "ok\n", "", 0},
	} {
		got := runShell(t, dir, tc.cmd)
		if got["stdout"] != tc.stdout || got["stderr"] != tc.stderr || got["exitCode"] != tc.code {
			t.Errorf("%s: stdout %q stderr %q exit %v, want %q %q %d", tc.cmd, got["stdout"], got["stderr"], got["exitCode"], tc.stdout, tc.stderr, tc.code)
		}
	}
}

// A ~ is the home directory, whichever shell expands it (recorded: runs/shell-glob-tilde).
func TestTildeIsTheHomeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got := runShell(t, t.TempDir(), `echo ~/x`)
	if got["stdout"] != home+"/x\n" {
		t.Errorf("stdout = %q", got["stdout"])
	}
}

// A line with no unquoted glob is run as written.
func TestACommandLineWithoutAGlobIsNotRewritten(t *testing.T) {
	for _, l := range []string{`echo "*" '?' ~`, `[ -f x ] && echo $((2*3))`, `for f in a b; do echo $f; done`} {
		if got := withNoMatchCheck(l); got != l {
			t.Errorf("%q rewritten to %q", l, got)
		}
	}
	if got := withNoMatchCheck(`a | b *.x > out`); !strings.Contains(got, "{ __nm 1 *.x && b *.x; } > out") {
		t.Errorf("rewritten to %q", got)
	}
}

// The word type of a bracket glob in the frame (recorded: runs/shell-glob-tilde).
func TestWordTypesOfGlobsAndTilde(t *testing.T) {
	p := frameArgs(t, `{"command":"echo *.txt ?.txt ~/x a~b [ab].txt x[1]y \"*\""}`)["parsingResult"].(map[string]any)
	var types []any
	for _, a := range p["executableCommands"].([]map[string]any)[0]["args"].([]any) {
		types = append(types, a.(map[string]any)["type"])
	}
	want := []any{"word", "word", "word", "word", "concatenation", "concatenation", "string"}
	if len(types) != len(want) {
		t.Fatalf("types = %v", types)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("types = %v, want %v", types, want)
		}
	}
}
