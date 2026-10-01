package subagents

import (
	"regexp"
	"strings"
)

// lineBreaks are what a hand-back normalises to "\n" before indenting: a
// carriage return, and the Unicode and control line separators.
var lineBreaks = regexp.MustCompile(`\r\n?|[\x{2028}\x{2029}\x{85}\x{0b}\x{0c}\x{1c}-\x{1e}]`)

// HandBack is the text a foreground sub-agent hands its parent in place of its
// report: the harness's frame, then the report with every line indented, so a
// frame-like line inside it cannot be mistaken for the frame; empty stands in
// when the sub-agent said nothing. A foreground sub-agent blocks its parent
// until it has finished, and the parent gets this as the tool's result.
//
// sr:capability foreground-subagent-result
func HandBack(frame, empty, report string) string {
	if report == "" {
		report = empty
	}
	norm := lineBreaks.ReplaceAllString(report, "\n")
	return frame + "\n  " + strings.ReplaceAll(norm, "\n", "\n  ")
}
