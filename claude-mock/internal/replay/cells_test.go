package replay

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// notAbout are the places a capability cell's claude text uses a word the replay
// leaves out of the comparison, and why that is not the cell's behaviour being
// left out ("cell/word": reason). An entry the cells no longer need fails the
// test, so the list only shrinks.
var notAbout = map[string]string{
	"background-agent/script":          "prose: a mock sub-agent is a scenario script",
	"session-fork/script":              "prose: the scenario script writes the init and result frames",
	"session-resume/script":            "prose: the scenario script writes the init frame",
	"session-fork/init":                "the cell declares the mock writes no init frame at a session's start (only one after a compaction), and both are left out of the comparison (frames.go unmodelled)",
	"session-resume/init":              "the cell declares the mock writes no init frame at a session's start (only one after a compaction), and both are left out of the comparison (frames.go unmodelled)",
	"foreground-subagent-result/usage": "the sub-agent's token counts, which the mock does not spend (a trailer's text is still compared)",
	"stop-block-continuation/caller":   "prose: the caller who reads the result",
}

// No capability cell may be about what the replay leaves out of the comparison:
// its statement and its deviations (the text the cells say their behaviour in)
// are searched for each dropped key and frame, bar the places notAbout explains.
func TestNoCellNamesWhatReplayDrops(t *testing.T) {
	keys, frames := Dropped()
	words := map[string]bool{}
	for _, k := range keys {
		words[k] = true
	}
	for _, f := range frames {
		words[strings.Split(strings.TrimSuffix(f, "/"), "/")[len(strings.Split(strings.TrimSuffix(f, "/"), "/"))-1]] = true
	}
	var names []string
	for w := range words {
		names = append(names, w)
	}
	sort.Strings(names)
	cells, err := filepath.Glob(filepath.Join("..", "..", "..", "spec", "capabilities", "*.yaml"))
	if err != nil || len(cells) == 0 {
		t.Fatalf("no capability cells found: %v", err)
	}
	used := map[string]bool{}
	for _, path := range cells {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var cell struct {
			Statement string
			Providers map[string]any
		}
		if err := yaml.Unmarshal(raw, &cell); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		claude, _ := cell.Providers["claude"].(map[string]any)
		if claude == nil {
			continue
		}
		text := cell.Statement
		devs, _ := claude["deviations"].([]any)
		for _, d := range devs {
			if m, ok := d.(map[string]any); ok {
				s, _ := m["statement"].(string)
				text += " " + s
			}
		}
		name := strings.TrimSuffix(filepath.Base(path), ".yaml")
		for _, w := range names {
			if !regexp.MustCompile(`(^|[^A-Za-z_])` + regexp.QuoteMeta(w) + `($|[^A-Za-z_])`).MatchString(text) {
				continue
			}
			if _, ok := notAbout[name+"/"+w]; ok {
				used[name+"/"+w] = true
				continue
			}
			t.Errorf("the cell %s names %q, which the claude replay leaves out of the comparison: replay it, or explain in notAbout", name, w)
		}
	}
	for k := range notAbout {
		if !used[k] {
			t.Errorf("notAbout lists %s, which no cell needs: remove it", k)
		}
	}
}
