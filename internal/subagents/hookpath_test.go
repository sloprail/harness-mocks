package subagents

import "testing"

func TestHookWorktreePath_LastNonEmptyLineWithoutEscapes(t *testing.T) {
	for out, want := range map[string]string{
		"/w/one\n":                         "/w/one",
		"banner\n\x1b[1m/w/two\x1b[0m\n\n": "/w/two",
		"  /w/three  ":                     "/w/three",
	} {
		if got, ok := hookWorktreePath(out); !ok || got != want {
			t.Fatalf("hookWorktreePath(%q) = %q, %v; want %q", out, got, ok, want)
		}
	}
	for _, out := range []string{"", "\n \n", "\x1b[0m"} {
		if got, ok := hookWorktreePath(out); ok {
			t.Fatalf("hookWorktreePath(%q) = %q, want no path", out, got)
		}
	}
}
