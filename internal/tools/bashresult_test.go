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
