package hooks

import (
	"regexp"
	"strings"
)

// MatcherStyle is how a harness reads a hook's matcher.
type MatcherStyle int

const (
	// MatchRegexp: any matcher other than the match-all ones is a regular
	// expression searched in the subject.
	MatchRegexp MatcherStyle = iota
	// MatchExactOrRegexp: a matcher of only letters, digits, "_", "-", spaces,
	// "," and "|" is an exact name, or a list of exact names separated by "|"
	// or ","; any other matcher is a regular expression searched in the
	// subject.
	MatchExactOrRegexp
)

// Matches reports whether a hook's matcher selects the value its event filters
// on (the tool name for a tool event), reading the matcher as a regular
// expression: see Select.
func Matches(matcher, value string, aliases ...string) bool {
	return Select(MatchRegexp, matcher, value, aliases...)
}

// Select reports whether a hook's matcher selects the subject its event
// filters on (the tool name for a tool event, how a session started for a
// session-start event): an empty matcher or "*" selects everything; any other
// is read as the style says, against the subject and each of its aliases
// (other names a harness gives the same tool). A matcher that is not a regular
// expression selects nothing.
//
// sr:capability hook-matcher-filter
func Select(style MatcherStyle, matcher, subject string, aliases ...string) bool {
	if matcher == "" || matcher == "*" {
		return true
	}
	names := append([]string{subject}, aliases...)
	if style == MatchExactOrRegexp && exactOnly(matcher) {
		for _, alt := range strings.FieldsFunc(matcher, func(r rune) bool { return r == '|' || r == ',' }) {
			for _, n := range names {
				if strings.TrimSpace(alt) == n {
					return true
				}
			}
		}
		return false
	}
	re, err := regexp.Compile(matcher)
	if err != nil {
		return false
	}
	for _, n := range names {
		if re.MatchString(n) {
			return true
		}
	}
	return false
}

// exactOnly is whether a matcher holds nothing but the characters of an exact
// name or a list of them.
func exactOnly(matcher string) bool {
	for _, r := range matcher {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_', r == '-', r == ' ', r == ',', r == '|':
		default:
			return false
		}
	}
	return true
}
