package hooks

import (
	"encoding/json"
	"strings"
)

// IsJSONOutput reports whether a hook command's stdout is read as JSON: it
// starts with { and ends with }, ignoring surrounding whitespace. Anything else
// is plain text, which some events take as context. Two or more lines that each
// parse as JSON on their own are plain text too, unless one of them is an
// object setting an output field (a harness names its fields).
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-0
func IsJSONOutput(stdout string, outputField func(key string) bool) bool {
	s := strings.TrimSpace(stdout)
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return false
	}
	lines := strings.Split(s, "\n")
	if len(lines) < 2 {
		return true
	}
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" && !json.Valid([]byte(l)) {
			return true // one JSON document over several lines
		}
	}
	for _, l := range lines {
		var obj map[string]json.RawMessage
		if json.Unmarshal([]byte(strings.TrimSpace(l)), &obj) == nil {
			for k := range obj {
				if outputField(k) {
					return true
				}
			}
		}
	}
	return false
}
