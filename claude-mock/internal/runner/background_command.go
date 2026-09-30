package runner

import (
	"encoding/json"
	"strings"
)

// runsInBackground reports whether a tool input asks to run in the background.
// Accepted as a JSON boolean or the string "true" (scenario builders that
// encode every input value as a string).
func runsInBackground(input json.RawMessage) bool {
	var in struct {
		RunInBackground any `json:"run_in_background"`
	}
	if json.Unmarshal(input, &in) != nil {
		return false
	}
	switch v := in.RunInBackground.(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}

// changesDirectory reports whether a command has a subcommand that changes
// directory (cd, pushd, popd, chdir) — when real Claude Code adds the
// "Session cwd remains …" note to a background receipt.
func changesDirectory(command string) bool {
	for _, part := range strings.FieldsFunc(command, func(r rune) bool {
		return r == ';' || r == '&' || r == '|' || r == '\n'
	}) {
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "cd", "pushd", "popd", "chdir":
			return true
		}
	}
	return false
}
