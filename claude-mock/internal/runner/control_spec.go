package runner

import (
	"strings"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

// defaultPreserved is how many of the last records a compaction keeps when
// the scenario does not say. Real compactions kept between 2 and 21 (every
// compact_boundary in the real transcripts); a manual /compact in a controlled
// claude 2.1.282 run kept 2.
const defaultPreserved = 2

// compactHookLines is what a compaction reports of its Pre/PostCompact
// hooks, one line per hook, in the 2.1.282 binary's wording:
// "<Event> [<command>] completed successfully[: <output>]" or
// "<Event> [<command>] failed[: <output>]".
func compactHookLines(event string, runs []hooks.HandlerRun) []string {
	var out []string
	for _, r := range runs {
		verdict, text := "completed successfully", strings.TrimSpace(r.Stdout)
		if r.ExitCode != 0 {
			verdict, text = "failed", strings.TrimSpace(r.Stderr)
		}
		l := event + " [" + r.Command + "] " + verdict
		if text != "" {
			l += ": " + text
		}
		out = append(out, l)
	}
	return out
}

// compactionSpec is what one compaction's boundary records.
type compactionSpec struct {
	logicalParent string
	preserve      int
	anchor        string
	trigger       string
	preTokens     int
	postTokens    int
	durationMs    int64
	withSegment   bool
	tailOffset    int
}
