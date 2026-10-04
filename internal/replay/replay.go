// Package replay compares what a mock produced with what a recording of the
// real harness shows. It knows nothing of any harness: a harness adapter turns
// a recording into the mock's script, runs the mock, and hands both outputs
// here as JSON lines together with Rules that name what two runs of the same
// behaviour may differ in. Everything not named is compared.
package replay

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Rules say what may differ between a recording and a replay of it. Each entry
// is a field that no capability cell is about, and the adapter that adds it
// says why.
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

// Sorted returns lines in a fixed order, for output whose order is not the
// behaviour (hooks that run at the same time).
func Sorted(lines []string) []string {
	out := append([]string(nil), lines...)
	sort.Strings(out)
	return out
}

// Diff is empty when want and got are the same lines; otherwise it shows the
// first line that differs with a little context, and the counts.
func Diff(what string, want, got []string) string {
	if len(want) == len(got) {
		same := true
		for i := range want {
			if want[i] != got[i] {
				same = false
				break
			}
		}
		if same {
			return ""
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: recording has %d lines, mock has %d\n", what, len(want), len(got))
	shown := 0
	for i := 0; i < len(want) || i < len(got) && shown < 3; i++ {
		var w, g string
		if i < len(want) {
			w = want[i]
		}
		if i < len(got) {
			g = got[i]
		}
		if w != g {
			fmt.Fprintf(&b, "line %d differs\n  recording: %s\n  mock:      %s\n", i+1, clip(w), clip(g))
			if shown++; shown == 3 {
				break
			}
		}
	}
	return b.String()
}

func clip(s string) string {
	if len(s) > 600 {
		return s[:600] + "…"
	}
	if s == "" {
		return "(none)"
	}
	return s
}
