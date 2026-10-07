package unimplemented

import "testing"

func TestIsNamesTheLongAndTheShortForms(t *testing.T) {
	for _, w := range [][]string{{"--output-schema", "s.json"}, {"-o", "f"}, {"--enable", "multi_agent_v2"}, {"--disable", "other"}} {
		if !Is(w, 0) {
			t.Errorf("%v is refused", w)
		}
	}
	for _, w := range [][]string{{"--json"}, {"-c"}, {"-C"}, {"-m"}, {"output-schema"}, {"--skip-git-repo-check"}, {"--sandbox", "read-only"}, {"-s", "read-only"},
		{"--ignore-user-config"}, {"--disable", "hooks"}, {"--enable", "hooks"}} {
		if Is(w, 0) {
			t.Errorf("%v is not refused", w)
		}
	}
}
