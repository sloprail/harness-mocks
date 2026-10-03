package runner

import (
	"encoding/json"
	"strings"
)

// stopsFor is whether a SubagentStop hook's JSON output says continue:false,
// which outranks the block of any other matching hook.
func stopsFor(stdout string) bool {
	var out struct {
		Continue *bool `json:"continue"`
	}
	return json.Unmarshal([]byte(strings.TrimSpace(stdout)), &out) == nil && out.Continue != nil && !*out.Continue
}
