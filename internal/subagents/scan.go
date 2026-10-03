package subagents

import "regexp"

// ScanRule is one pattern of a harness's sub-agent output scan: text that
// imitates the harness's own output or names a permission setting.
type ScanRule struct {
	// Name is how the harness names the pattern when it reports it.
	Name string
	// Match finds the pattern.
	Match *regexp.Regexp
	// Rewrite, when not empty, is the expansion (as regexp.Expand) a match is
	// rewritten to, to neutralise it; empty leaves the text as written.
	Rewrite string
	// Report is whether a match is reported by name: some patterns are only
	// neutralised and some only reported.
	Report bool
}

// Scan is a harness's scan of a sub-agent's final report before its parent
// reads it: it never removes or rewords anything, it only rewrites what
// imitates the harness's own output so it reads as ordinary text, and lists
// the reported patterns that matched, in the rules' order.
//
// sr:capability foreground-subagent-result
func Scan(report string, rules []ScanRule) (scanned string, matched []string) {
	scanned = report
	for _, r := range rules {
		if !r.Match.MatchString(scanned) {
			continue
		}
		if r.Report {
			matched = append(matched, r.Name)
		}
		if r.Rewrite != "" {
			scanned = r.Match.ReplaceAllString(scanned, r.Rewrite)
		}
	}
	return scanned, matched
}
