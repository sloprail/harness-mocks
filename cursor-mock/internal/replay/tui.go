package replay

import (
	"os"
	"regexp"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// An interactive run (recorded: runs/tui-*) is cursor-agent's TUI played by tools/tui-record from
// the setup's tui.yaml. It has no stream, so a replay compares the hook payloads alone; the
// mock is run without -p, the prompt on its stdin, as a user types it.
const tuiCommand = "cursor-agent --trust --model auto"

// isTUI reports whether the recorded run is a TUI session.
func isTUI(setup map[string]string) bool {
	_, ok := setup["tui.yaml"]
	return ok
}

// commandOK reports whether the command run.yaml records is the one this adapter replays:
// the print command with or without --force (as the setup's no-force file says), or the TUI's.
func commandOK(setup map[string]string, command string) bool {
	if isTUI(setup) {
		return command == tuiCommand
	}
	_, noForce := setup["no-force"]
	return noForce == (command == unforcedCommand) && (command == forcedCommand || command == unforcedCommand)
}

// untranscribed is the recording of a TUI session whose prompt a hook refused (recorded:
// runs/tui-prompt-blocked): the agent never ran, so there is no transcript and no turn of the
// model to tell, only the hook payloads to compare.
func untranscribed(rec core.Recording, dir string) (core.Recording, bool) {
	kept, _ := os.ReadDir(dir)
	return rec, isTUI(rec.Setup) && len(kept) == 0
}

var typedCommand = regexp.MustCompile(`(?m)send: \{text: "(/[a-z]+)"`)

// typedInput is what the user types in the TUI session: the prompt, then each slash command
// the setup's tui.yaml sends at the idle input after it (recorded: runs/tui-manual-compaction).
func typedInput(setup map[string]string, prompt string) []byte {
	lines := []string{prompt}
	for _, m := range typedCommand.FindAllStringSubmatch(setup["tui.yaml"], -1) {
		lines = append(lines, m[1])
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

// sessionOfRun is the session the run's stream names, or, when it has none (a TUI), its first
// hook payload.
func sessionOfRun(stream, payloads []map[string]any) string {
	if id := sessionOf(stream); id != "" {
		return id
	}
	return sessionOf(payloads)
}
