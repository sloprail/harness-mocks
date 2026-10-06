package unimplemented

import "testing"

func TestIsNamesTheLongAndTheShortForms(t *testing.T) {
	for _, w := range []string{"--output-schema", "-o", "--sandbox", "-s", "--enable"} {
		if !Is(w) {
			t.Errorf("%s is refused", w)
		}
	}
	for _, w := range []string{"--json", "-c", "-C", "-m", "output-schema", "--skip-git-repo-check"} {
		if Is(w) {
			t.Errorf("%s is not refused", w)
		}
	}
}
