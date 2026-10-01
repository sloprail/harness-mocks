package hooks

import "strings"

// IsJSONOutput reports whether a hook command's stdout is read as JSON: it
// starts with { and ends with }, ignoring surrounding whitespace. Anything else
// is plain text, which some events take as context.
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-0
func IsJSONOutput(stdout string) bool {
	s := strings.TrimSpace(stdout)
	return strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")
}
