// Package replaytest holds what the adapters' replay tests share: the check
// that no capability cell is about what a replay drops from its comparison.
package replaytest

import (
	"os"
	"path/filepath"
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
	bad := make([]bool, len(excuses)) // the excuses already refused, once each
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
			for _, m := range occurrences(text, strings.ToLower(k)) {
				start, end := m[0], m[1]
				excused := false
				for i, e := range excuses {
					if e.Cell != name || strings.ToLower(e.Key) != strings.ToLower(k) {
						continue
					}
					et := fold(e.Text)
					if n := len(occurrences(et, strings.ToLower(k))); n != 1 {
						if !bad[i] {
							bad[i] = true
							t.Errorf("excuse %s/%s: the text %q names the key %d times, it must name it once (it excuses one occurrence)", e.Cell, e.Key, e.Text, n)
						}
						used[i] = true
						continue
					}
					if n := strings.Count(text, et); n != 1 {
						if !bad[i] {
							bad[i] = true
							t.Errorf("excuse %s/%s: the text %q occurs %d times in the cell, it must occur once: lengthen it", e.Cell, e.Key, e.Text, n)
						}
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

// occurrences are the [start, end) offsets of key in text as a whole word: not
// next to a letter, digit or underscore. Overlapping is impossible, and two
// occurrences may share a separator ("model model").
func occurrences(text, key string) [][2]int {
	var out [][2]int
	word := func(i int) bool {
		if i < 0 || i >= len(text) {
			return false
		}
		c := text[i]
		return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z'
	}
	for from := 0; ; {
		i := strings.Index(text[from:], key)
		if i < 0 {
			return out
		}
		i += from
		if !word(i-1) && !word(i+len(key)) {
			out = append(out, [2]int{i, i + len(key)})
		}
		from = i + 1
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
