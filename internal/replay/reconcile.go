package replay

// Reconcile is the mock's per-sample output, each as it is to be compared with
// the sample it was replayed against, with one allowance: where the recorded
// samples themselves disagree on a line (the real harness races, and two
// captures of one run show different outcomes), the mock's line is accepted if it
// is the line of one of the samples. Everything else is compared as it is: a
// line the samples agree on, or a mock line that no sample shows, is a
// difference. wants[i] and gots[i] are sample i's recording and the mock's run of it.
func Reconcile(wants, gots []Observed) []Observed {
	out := make([]Observed, len(gots))
	for i := range gots {
		out[i] = Observed{Events: reconcileLines(i, linesOf(wants, true), gots[i].Events), Hooks: reconcileLines(i, linesOf(wants, false), gots[i].Hooks)}
	}
	return out
}

func linesOf(wants []Observed, events bool) [][]string {
	out := make([][]string, len(wants))
	for i, w := range wants {
		if events {
			out[i] = w.Events
		} else {
			out[i] = w.Hooks
		}
	}
	return out
}

// reconcileLines is sample i's mock lines with each line at which the samples
// disagree and which equals another sample's line there replaced by this sample's
// own, which makes it equal to what the diff compares it with.
func reconcileLines(i int, wants [][]string, got []string) []string {
	out := append([]string(nil), got...)
	for p := range out {
		if p >= len(wants[i]) || out[p] == wants[i][p] {
			continue
		}
		for j := range wants {
			if j != i && p < len(wants[j]) && wants[j][p] == out[p] && racy(wants, p) {
				out[p] = wants[i][p]
				break
			}
		}
	}
	return out
}

// racy is whether the samples disagree on line p (those that have one).
func racy(wants [][]string, p int) bool {
	var first string
	seen := false
	for _, w := range wants {
		if p >= len(w) {
			continue
		}
		if !seen {
			first, seen = w[p], true
		} else if w[p] != first {
			return true
		}
	}
	return false
}
