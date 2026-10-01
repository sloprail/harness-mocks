package tools

import "testing"

func TestBashMessage(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    BashResult
		text string
		err  bool
	}{
		{"output, trailing newlines trimmed", BashResult{Output: "hello\n\n"}, "hello", false},
		{"no output", BashResult{}, "", false},
		{"failed with output", BashResult{Output: "OUT-LINE\nERR-LINE\n", ExitCode: 3}, "Exit code 3\nOUT-LINE\nERR-LINE", true},
		{"failed with none", BashResult{ExitCode: 1}, "Exit code 1", true},
		{"a shell that did not start", BashResult{Output: "no such shell", ExitCode: -1}, "Exit code -1\nno such shell", true},
		{"inner blank lines stay", BashResult{Output: "a\n\nb\n"}, "a\n\nb", false},
	} {
		if text, isErr := tc.r.Message(); text != tc.text || isErr != tc.err {
			t.Errorf("%s: Message = (%q, %v), want (%q, %v)", tc.name, text, isErr, tc.text, tc.err)
		}
	}
}

func TestBashMessageForBenignExit1(t *testing.T) {
	benign := []string{"grep", "rg", "find", "diff", "test", "[", "git diff", "git grep"}
	for _, tc := range []struct {
		command string
		r       BashResult
		text    string
		err     bool
	}{
		{"grep foo file", BashResult{ExitCode: 1}, "", false},
		{"cat file | grep foo", BashResult{ExitCode: 1, Output: "x\n"}, "x", false},
		{"FOO=bar grep foo file", BashResult{ExitCode: 1}, "", false},
		{"git diff --quiet", BashResult{ExitCode: 1}, "", false},
		{"git status", BashResult{ExitCode: 1, Output: "no\n"}, "Exit code 1\nno", true},
		{"pgrep foo", BashResult{ExitCode: 1}, "Exit code 1", true},
		{"cmp a b", BashResult{ExitCode: 1}, "Exit code 1", true},
		{"grep foo file", BashResult{ExitCode: 2, Output: "bad\n"}, "Exit code 2\nbad", true},
		{"grep foo file", BashResult{ExitCode: 0, Output: "ok\n"}, "ok", false},
		{"grep foo file; false", BashResult{ExitCode: 1}, "Exit code 1", true},
		{"", BashResult{ExitCode: 1}, "Exit code 1", true},
	} {
		if text, isErr := tc.r.MessageFor(tc.command, benign); text != tc.text || isErr != tc.err {
			t.Errorf("%q: MessageFor = (%q, %v), want (%q, %v)", tc.command, text, isErr, tc.text, tc.err)
		}
	}
}
