package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded runs runs/add-dir-access and runs/no-add-dir-access: the same
// three Reads (a file in a sibling directory, a file in another directory
// outside every root, and a file of the project), the first run with that
// sibling added as a second root (--add-dir ../second-root), the second without.

// TestAddingARootChangesNeitherWhatARunMayReadNorWhatItsHooksSee: recorded, a
// headless run (--force) reads a file of the added directory, a file outside
// every root and a file of the project alike, with or without --add-dir: the
// three reads all succeed, each with its beforeReadFile and postToolUse, and
// the hooks name the project as the one workspace root. The mock accepts
// --add-dir and does the same, which is why it can ignore it.
// sr:proves file-tools/cursor
// sr:proves hook-common-payload/cursor
func TestAddingARootChangesNeitherWhatARunMayReadNorWhatItsHooksSee(t *testing.T) {
	for run, args := range map[string][]string{
		"add-dir-access":    {"--add-dir", "../second-root"},
		"no-add-dir-access": nil,
	} {
		got, want := replayWith(t, run, args...)
		conforms(t, got, want)
		for name, o := range map[string]observed{"recorded": want, "mock": got} {
			var before, after int
			for _, h := range o.hooks {
				switch h["hook_event_name"] {
				case "beforeReadFile":
					before++
				case "postToolUse":
					after++
				case "postToolUseFailure":
					t.Fatalf("%s %s: a read failed", run, name)
				}
			}
			require.Equal(t, []int{3, 3}, []int{before, after}, run+" "+name)
			require.Equal(t, []string{"tool_call/completed/readToolCall/success", "tool_call/completed/readToolCall/success", "tool_call/completed/readToolCall/success"}, completedReads(o.frames), run+" "+name)
		}
	}
}
