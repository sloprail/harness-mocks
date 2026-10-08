package replay

import (
	"os"
	"strings"

	"go.yaml.in/yaml/v3"

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

// nameIDsOfRun is nameHookIDs for a run with a stream. A TUI has none to tell a call's own id from
// the one its hooks name it by: the hooks' ids are compared by the order they appear in.
func nameIDsOfRun(rec *core.Recording, stream, payloads []map[string]any, session string) error {
	if isTUI(rec.Setup) {
		return nil
	}
	return nameHookIDs(&rec.Agent, stream, payloads, session)
}

// untranscribed is the recording of a TUI session whose prompt a hook refused (recorded:
// runs/tui-prompt-blocked): the agent never ran, so there is no transcript and no turn of the
// model to tell, only the hook payloads to compare.
func untranscribed(rec core.Recording, dir string) (core.Recording, bool) {
	kept, _ := os.ReadDir(dir)
	return rec, isTUI(rec.Setup) && len(kept) == 0
}

// typedInput is what the user types in the TUI session, a line each, in the order the setup's
// tui.yaml sends them: its prompts (recorded: runs/tui-multi-turn) and the slash commands typed at
// the idle input (runs/tui-manual-compaction). A step's text is its `text`, or the content of
// `text_file`, which the setup holds only as prompt.txt. Each send ends in a key (enter), so one
// is one line; a text of several lines would not be.
func typedInput(setup map[string]string) ([]byte, error) {
	var script struct {
		Steps []struct {
			Send *struct {
				Text     string   `yaml:"text"`
				TextFile string   `yaml:"text_file"`
				Keys     []string `yaml:"keys"`
			} `yaml:"send"`
		} `yaml:"steps"`
	}
	if err := yaml.Unmarshal([]byte(setup["tui.yaml"]), &script); err != nil {
		return nil, unbuildable("tui.yaml: %v", err)
	}
	var lines []string
	for _, st := range script.Steps {
		if st.Send == nil || st.Send.Text == "" && st.Send.TextFile == "" {
			continue
		}
		text := st.Send.Text
		if st.Send.TextFile != "" {
			if st.Send.TextFile != "prompt.txt" {
				return nil, unbuildable("tui.yaml types the content of %s, which the adapter does not install", st.Send.TextFile)
			}
			text = strings.TrimRight(setup["prompt.txt"], "\n")
		}
		if strings.Contains(text, "\n") || !contains(st.Send.Keys, "enter") {
			return nil, unbuildable("tui.yaml types %q, which is not one line submitted with enter", text)
		}
		lines = append(lines, text)
	}
	if len(lines) == 0 {
		return nil, unbuildable("tui.yaml types nothing")
	}
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

// sessionOfRun is the session the run's stream names, or, when it has none (a TUI), its first
// hook payload.
func sessionOfRun(stream, payloads []map[string]any) string {
	if id := sessionOf(stream); id != "" {
		return id
	}
	return sessionOf(payloads)
}
