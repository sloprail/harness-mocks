package replay

// canon.go is a toolkit for an adapter's normalisation: the mechanism of
// dropping, rewriting, scrubbing and renumbering, with no value of its own. What
// to drop, rewrite or scrub is a harness's knowledge and lives in its adapter.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
)

// Rules say what may differ between a recording and a replay of it. The
// adapter that fills them in says why each entry is not behaviour.
type Rules struct {
	// DropKeys are object keys removed wherever they occur, though no cell is about them.
	DropKeys []string
	// MaskKeys are object keys whose value differs per run but whose presence is
	// behaviour (a transcript path, a token count): the key stays, its value is
	// replaced by <key>, so a payload that lacks it is a difference. A key in both
	// lists is dropped; a masked key's Rewrite does not run.
	MaskKeys []string
	// Scrub rewrites every string value (a path, a pid, a duration).
	Scrub []Scrub
	// Rewrite changes the string value of the named key wherever it occurs (a
	// command line that differs only in how the shell was invoked).
	Rewrite map[string]func(string) string
	// Measured are object keys whose number is a measurement that differs in every
	// run (how long a call took): it is compared by what it says of the run, not
	// its value, as "<zero>" or "<positive>".
	Measured []string
	// IDs are patterns of values that differ per run but must agree with
	// themselves: each distinct match is renamed <ID1>, <ID2>... in order of
	// first appearance, so a line that names the same id as an earlier one must
	// still do so.
	IDs []*regexp.Regexp
}

// Scrub is one rewrite of the text of a string value.
type Scrub struct {
	Re   *regexp.Regexp
	With string
}

// Canon canonicalises the outputs of one run under Rules. The id numbering is
// shared by everything it canonicalises, so call Lines for the stream that
// names ids in a fixed order (the event stream) before the ones whose order
// is not fixed (hook payloads).
type Canon struct{ c canon }

// New makes a Canon for one run's outputs.
func New(r Rules) *Canon {
	return &Canon{canon{r: r, ids: map[string]string{}}}
}

// Lines canonicalises JSON objects: one string per object, keys sorted, rules applied.
func (n *Canon) Lines(objs []map[string]any) []string {
	out := make([]string, len(objs))
	for i, o := range objs {
		b, _ := json.Marshal(n.c.walk(o))
		out[i] = string(b)
	}
	return out
}

type canon struct {
	r   Rules
	ids map[string]string
}

func (c *canon) walk(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		// in key order, so that ids are numbered in an order that is the same in every run
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	next:
		for _, k := range keys {
			e := x[k]
			for _, d := range c.r.DropKeys {
				if k == d {
					continue next
				}
			}
			if slices.Contains(c.r.MaskKeys, k) {
				out[c.str(k)] = "<" + k + ">"
				continue
			}
			if fn := c.r.Rewrite[k]; fn != nil {
				if str, ok := e.(string); ok {
					e = fn(str)
				}
			}
			if n, ok := e.(float64); ok && c.measured(k) {
				e = Measure(n)
			}
			out[c.str(k)] = c.walk(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = c.walk(e)
		}
		return out
	case string:
		return c.str(x)
	}
	return v
}

func (c *canon) measured(key string) bool {
	for _, m := range c.r.Measured {
		if m == key {
			return true
		}
	}
	return false
}

// Measure is what a measurement says without its value: whether it is zero.
func Measure(n float64) string {
	if n == 0 {
		return "<zero>"
	}
	return "<positive>"
}

// str applies the scrubs and the id renaming to a string (a value, or a key).
func (c *canon) str(x string) string {
	for _, s := range c.r.Scrub {
		x = s.Re.ReplaceAllString(x, s.With)
	}
	for _, re := range c.r.IDs {
		x = re.ReplaceAllStringFunc(x, c.name)
	}
	return x
}

// name is the id's stable name: <ID1>, <ID2>... in order of first appearance.
func (c *canon) name(id string) string {
	if n, ok := c.ids[id]; ok {
		return n
	}
	n := fmt.Sprintf("<ID%d>", len(c.ids)+1)
	c.ids[id] = n
	return n
}
