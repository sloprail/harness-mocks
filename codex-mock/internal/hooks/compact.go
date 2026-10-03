package hooks

import (
	"encoding/json"
	"strings"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// Stops reports whether a compaction hook's output stops the compaction: it
// exits 0 and prints JSON whose `continue` is false. Plain text, any other
// output and a hook that failed to run are ignored.
//
// sr:docs https://developers.openai.com/codex/hooks#precompact
func Stops(o corehooks.Outcome) bool {
	if !o.Started || o.TimedOut || o.Exit != 0 {
		return false
	}
	var out struct {
		Continue *bool `json:"continue"`
	}
	text := strings.TrimSpace(o.Stdout)
	if !strings.HasPrefix(text, "{") || json.Unmarshal([]byte(text), &out) != nil {
		return false
	}
	return out.Continue != nil && !*out.Continue
}
