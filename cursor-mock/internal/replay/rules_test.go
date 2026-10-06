package replay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"go.yaml.in/yaml/v3"
)

// What a replay does not compare must be nothing a capability cell is about
// (adr/capability-grounding): no key the rules drop, no frame type or hook event
// the adapter leaves out, may be named in a cursor cell's statement or in a
// deviation of the cell that does not declare it unmodelled.
func TestNothingDroppedIsWhatACellIsAbout(t *testing.T) {
	dropped := append([]string{}, dropKeys...)
	for typ := range unmodelled {
		dropped = append(dropped, typ)
	}
	about := map[string]string{} // word -> the cell naming it
	cells, err := filepath.Glob(filepath.Join("..", "..", "..", "spec", "capabilities", "*.yaml"))
	if err != nil || len(cells) == 0 {
		t.Fatalf("no capability cells found: %v", err)
	}
	for _, path := range cells {
		var cell struct {
			Statement string `yaml:"statement"`
			Providers struct {
				Cursor struct {
					Deviations []struct {
						Kind      string `yaml:"kind"`
						Statement string `yaml:"statement"`
					} `yaml:"deviations"`
				} `yaml:"cursor"`
			} `yaml:"providers"`
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := yaml.Unmarshal(b, &cell); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		text := cell.Statement
		for _, d := range cell.Providers.Cursor.Deviations {
			if d.Kind != "mock-not-modeled" { // a cell that says the mock does not model it is not asking for it
				text += " " + d.Statement
			}
		}
		for _, w := range strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' }) {
			about[w] = filepath.Base(path)
		}
	}
	// words a cell uses in another sense than the key's
	exempt := map[string]string{"usage": "foreground-subagent-result.yaml speaks of the sub-agent's usage trailer, not of the result frame's token counts"}
	for _, key := range dropped {
		if _, ok := exempt[key]; ok {
			continue
		}
		if cell, named := about[key]; named {
			t.Errorf("%q is dropped by the replay and named by %s: compare it, or scrub it", key, cell)
		}
	}
}

// A measurement is compared by whether it is zero, not by its value.
func TestMeasuredDurationsAreComparedBySign(t *testing.T) {
	if got := measuredText("2890"); got != "<positive>" {
		t.Errorf("measuredText(2890) = %q", got)
	}
	if got := measuredText("0"); got != "<zero>" {
		t.Errorf("measuredText(0) = %q", got)
	}
	if got := measuredText("n/a"); got != "n/a" {
		t.Errorf("a string that is not a number is left alone, got %q", got)
	}
}
