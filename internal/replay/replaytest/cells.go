// Package replaytest holds what the adapters' replay tests share: the check
// that no capability cell is about what a replay drops from its comparison.
package replaytest

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Excuse says that one occurrence of a dropped key's word in a cell's text is
// prose, not the key: Text is the exact words around it (compared without
// regard to case or line breaks), and must contain the key and occur exactly
// once in the cell's text for the harness, so another use of the word, added
// later, is not excused by it.
type Excuse struct {
	Cell, Key, Text, Why string
}

// TB is the part of *testing.T the check uses.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// NoCellNames fails for every capability cell whose text for the harness
// (spec/capabilities/*.yaml under repoRoot: the cell's statement and everything
// its harness section says, bar the doc links and the recorded runs' paths)
// uses a word of keys, case aside, that no Excuse explains; and for every
// Excuse the cells no longer need or that is ambiguous.
// sr:invariant replay-fidelity
func NoCellNames(t TB, repoRoot, harness string, keys []string, excuses []Excuse) {
	t.Helper()
	cells, err := filepath.Glob(filepath.Join(repoRoot, "spec", "capabilities", "*.yaml"))
	if err != nil || len(cells) == 0 {
		t.Fatalf("no capability cells found: %v", err)
	}
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	used := make([]bool, len(excuses))
	for _, path := range cells {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%v", err)
		}
		var cell map[string]any
		if err := yaml.Unmarshal(raw, &cell); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		name := strings.TrimSuffix(filepath.Base(path), ".yaml")
		providers, _ := cell["providers"].(map[string]any)
		section, ok := providers[harness]
		if !ok {
			continue
		}
		var parts []string
		collect(cell["statement"], &parts)
		collect(section, &parts)
		text := fold(strings.Join(parts, "\n"))
		for _, k := range sorted {
			re := regexp.MustCompile(`(^|[^a-z0-9_])(` + regexp.QuoteMeta(strings.ToLower(k)) + `)($|[^a-z0-9_])`)
			for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
				start, end := m[4], m[5]
				excused := false
				for i, e := range excuses {
					if e.Cell != name || strings.ToLower(e.Key) != strings.ToLower(k) {
						continue
					}
					et := fold(e.Text)
					if !strings.Contains(et, strings.ToLower(k)) {
						t.Errorf("excuse %s/%s: the text %q does not contain the key", e.Cell, e.Key, e.Text)
						used[i] = true
						continue
					}
					if n := strings.Count(text, et); n != 1 {
						t.Errorf("excuse %s/%s: the text %q occurs %d times in the cell, it must occur once: lengthen it", e.Cell, e.Key, e.Text, n)
						used[i] = true
						continue
					}
					at := strings.Index(text, et)
					if at <= start && end <= at+len(et) {
						excused, used[i] = true, true
					}
				}
				if !excused {
					lo, hi := max(0, start-40), min(len(text), end+40)
					t.Errorf("the cell %s names %q (\"...%s...\"), which the %s replay drops from the comparison: keep the key behind a placeholder, or excuse this occurrence as prose", name, k, text[lo:hi], harness)
				}
			}
		}
	}
	for i, e := range excuses {
		if !used[i] {
			t.Errorf("the excuse %s/%s %q is not needed by any cell: remove it", e.Cell, e.Key, e.Text)
		}
	}
}

// fold lowercases and folds all whitespace to single spaces.
func fold(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

// collect appends every string under v, bar the values of "docs" and "runs".
func collect(v any, out *[]string) {
	switch x := v.(type) {
	case string:
		*out = append(*out, x)
	case []any:
		for _, e := range x {
			collect(e, out)
		}
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			if k != "docs" && k != "runs" {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			collect(x[k], out)
		}
	}
}
