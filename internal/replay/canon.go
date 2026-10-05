package replay

// canon.go is a toolkit for an adapter's normalisation: the mechanism of
// dropping, rewriting, scrubbing and renumbering, with no value of its own. What
// to drop, rewrite or scrub is a harness's knowledge and lives in its adapter.

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// Rules say what may differ between a recording and a replay of it. The
// adapter that fills them in says why each entry is not behaviour.
type Rules struct {
	// DropKeys are object keys removed wherever they occur (a timestamp, a token count).
	DropKeys []string
	// Scrub rewrites every string value (a path, a pid, a duration).
	Scrub []Scrub
	// Rewrite changes the string value of the named key wherever it occurs (a
	// command line that differs only in how the shell was invoked).
	Rewrite map[string]func(string) string
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
func New(r Rules) *Canon { return &Canon{canon{r: r, ids: map[string]string{}}} }

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
	keys:
		for k, e := range x {
			for _, d := range c.r.DropKeys {
				if k == d {
					continue keys
				}
			}
			if fn := c.r.Rewrite[k]; fn != nil {
				if str, ok := e.(string); ok {
					e = fn(str)
				}
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

// str applies the scrubs and the id renaming to a string (a value, or a key).
func (c *canon) str(x string) string {
	for _, s := range c.r.Scrub {
		x = s.Re.ReplaceAllString(x, s.With)
	}
	for _, re := range c.r.IDs {
		x = re.ReplaceAllStringFunc(x, func(m string) string {
			if n, ok := c.ids[m]; ok {
				return n
			}
			n := fmt.Sprintf("<ID%d>", len(c.ids)+1)
			c.ids[m] = n
			return n
		})
	}
	return x
}
