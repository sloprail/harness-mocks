package replaytest

import (
	"sort"
	"strings"
)

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
