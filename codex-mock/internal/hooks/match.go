package hooks

import "regexp"

// toolAliases are the other names a canonical tool name also matches.
var toolAliases = map[string][]string{"apply_patch": {"Edit", "Write"}}

// Matches reports whether a group's matcher selects the value an event
// filters on (the tool name for a tool event): "", "*" or no matcher match
// everything, any other matcher is a regular expression searched in the value
// or in one of its aliases. A matcher that is not a regular expression
// matches nothing.
//
// sr:docs https://developers.openai.com/codex/hooks#matcher-patterns
func Matches(matcher, value string) bool {
	if matcher == "" || matcher == "*" {
		return true
	}
	re, err := regexp.Compile(matcher)
	if err != nil {
		return false
	}
	for _, v := range append([]string{value}, toolAliases[value]...) {
		if re.MatchString(v) {
			return true
		}
	}
	return false
}

// UnfilteredEvent reports whether Codex ignores a matcher for the event.
func UnfilteredEvent(ev Event) bool { return ev == UserPromptSubmit || ev == Stop }
