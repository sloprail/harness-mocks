package replay

import (
	"fmt"
	"strings"
)

// Diff is empty when want and got are the same lines; otherwise it shows the
// first lines that differ and the counts.
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
			if shown++; shown == 30 {
				break
			}
		}
	}
	return b.String()
}

func clip(s string) string {
	if len(s) > 110 {
		return s[:110] + "…"
	}
	if s == "" {
		return "(none)"
	}
	return s
}
