package main

import (
	"fmt"
	"io"
	"strings"
)

const maxDiffItems = 25 // items shown per side before truncating
const maxItemLines = 10 // lines shown per item

func indent(it item, sign string) string {
	lines := strings.Split(strings.TrimRight(it.Text, "\n"), "\n")
	head := fmt.Sprintf("%s [%s] %s", sign, it.File, it.Kind)
	if len(lines) > maxItemLines {
		lines = append(lines[:maxItemLines], fmt.Sprintf("... (%d more lines)", len(lines)-maxItemLines))
	}
	return head + "\n" + sign + " " + strings.Join(lines, "\n"+sign+" ")
}

func printItems(w io.Writer, items []item, sign string) {
	for i, it := range items {
		if i == maxDiffItems {
			fmt.Fprintf(w, "%s ... %d more items\n", sign, len(items)-maxDiffItems)
			return
		}
		fmt.Fprintln(w, indent(it, sign))
	}
}

// report prints one line per package (plus a diff for violations) and
// returns whether the outcome passed. prefix labels each line, for --each.
func report(w io.Writer, prefix string, o outcome) bool {
	if len(o.nonGo) > 0 {
		fmt.Fprintf(w, "%sMOVED-ONLY VIOLATION non-.go files changed:\n", prefix)
		for _, f := range o.nonGo {
			fmt.Fprintf(w, "%s  %s\n", prefix, f)
		}
	}
	for _, p := range o.pkgs {
		if p.ok() {
			fmt.Fprintf(w, "%sok %s (%d items)\n", prefix, p.dir, p.count)
			continue
		}
		fmt.Fprintf(w, "%sMOVED-ONLY VIOLATION %s\n", prefix, p.dir)
		for _, n := range p.notes {
			fmt.Fprintf(w, "%s  note: %s\n", prefix, n)
		}
		fmt.Fprintf(w, "--- base\n+++ head\n")
		printItems(w, p.onlyBase, "-")
		printItems(w, p.onlyHead, "+")
	}
	return o.ok()
}
