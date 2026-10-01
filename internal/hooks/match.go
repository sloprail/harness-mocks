package hooks

import "regexp"

// Matches reports whether a hook's matcher selects the value its event filters
// on (the tool name for a tool event): an empty matcher or "*" selects
// everything; any other is a regular expression searched in value and in each
// of its aliases (other names a harness gives the same tool). A matcher that is
// not a regular expression selects nothing.
func Matches(matcher, value string, aliases ...string) bool {
	if matcher == "" || matcher == "*" {
		return true
	}
	re, err := regexp.Compile(matcher)
	if err != nil {
		return false
	}
	for _, v := range append([]string{value}, aliases...) {
		if re.MatchString(v) {
			return true
		}
	}
	return false
}
