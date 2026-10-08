package replay

import (
	"encoding/json"
	"strings"
)

// modelledFlags are the cursor-agent flags a recording's setup/args may hold
// that the mock models, and whether each takes a value (on the next line). Any
// other flag is something of the recording the adapter does not pass on, so the
// recording is not replayed rather than replayed without it.
var modelledFlags = map[string]bool{
	"--add-dir":      true,
	"--approve-mcps": false,
	"--print":        false,
	"--plugin-dir":   true,
	"--force":        false,
	"--resume":       true,
	"--model":        true,
}

// flagWords is the command-line words of a setup/args file, one word per line: each
// flag the mock models, with its value where it takes one. A word that is not a
// modelled flag, or a flag without its value, is not replayable. The args of a later
// step may name the first step's session (<SESSION>) and any flag, which the mock
// then refuses or not as it does the real command line.
func flagWords(args string, laterStep bool) ([]string, error) {
	var out []string
	lines := strings.Split(strings.TrimRight(args, "\n"), "\n")
	for i := 0; i < len(lines) && args != ""; i++ {
		takesValue, known := modelledFlags[lines[i]]
		if !known && laterStep && strings.HasPrefix(lines[i], "-") {
			// a later step is passed what it was given: a flag the mock does not model is
			// refused by the mock, and the status it refuses with is compared with the
			// recording's (recorded: runs/session-fork, a --fork that cursor-agent refuses)
			out = append(out, lines[i])
			continue
		}
		if !known {
			return nil, unbuildable("the setup's args hold %q, which the mock does not model", lines[i])
		}
		out = append(out, lines[i])
		if takesValue {
			if i+1 >= len(lines) || strings.HasPrefix(lines[i+1], "-") || lines[i+1] == "<SESSION>" && !laterStep {
				return nil, unbuildable("the setup's args give %s no value the adapter can pass: a session of an earlier step is not replayed", lines[i])
			}
			i++
			out = append(out, lines[i])
		}
	}
	return out, nil
}

// hooksConfigured reports whether the setup's project or user hooks.json configures
// at least one hook.
func hooksConfigured(setup map[string]string) bool {
	for _, name := range []string{"hooks.json", "user-hooks.json"} {
		var file struct {
			Hooks map[string][]json.RawMessage `json:"hooks"`
		}
		if json.Unmarshal([]byte(setup[name]), &file) != nil {
			continue
		}
		for _, entries := range file.Hooks {
			if len(entries) > 0 {
				return true
			}
		}
	}
	return false
}
