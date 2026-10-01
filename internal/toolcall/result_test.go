package toolcall

import "testing"

func TestResultText(t *testing.T) {
	for _, tc := range []struct{ tool, out, want string }{
		{"Bash", "", "(Bash completed with no output)"},
		{"Read", " \n\t", "(Read completed with no output)"},
		{"Bash", "hi", "hi"},
		{"Bash", "  padded  ", "  padded  "},
		{"Glob", "\n", "(Glob completed with no output)"},
	} {
		if got := ResultText(tc.tool, tc.out); got != tc.want {
			t.Errorf("ResultText(%q, %q) = %q, want %q", tc.tool, tc.out, got, tc.want)
		}
	}
}
