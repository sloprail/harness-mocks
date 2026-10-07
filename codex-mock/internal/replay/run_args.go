package replay

import (
	"fmt"
	"path/filepath"
	"strings"
)

// runFlags are the options of a recorded run (setup/args, one word per line) that the mock is given.
// A token limit that made the harness compact is dropped: the mock has no tokens, so the script
// compacts where the harness did (the rollout's compacted records). A recording made with an option
// the adapter cannot give the mock is not replayed.
func runFlags(args string) (pass []string, err error) {
	words := strings.Fields(args)
	for i := 0; i < len(words); i++ {
		w := words[i]
		next := ""
		if i+1 < len(words) {
			next = words[i+1]
		}
		switch {
		case w == "--ephemeral", w == "--ignore-user-config":
			pass = append(pass, w)
		case (w == "--disable" || w == "--enable") && next == "hooks", w == "-s" && sandboxMode(next):
			pass = append(pass, w, next)
			i++
		case w == "-c" && strings.HasPrefix(next, "model_auto_compact_token_limit="):
			i++
		case w == "-c" && strings.HasPrefix(next, "agents.max_depth="), w == "-C" && next != "", w == "resume" && next != "":
			pass = append(pass, w, next)
			i++
		default:
			return nil, fmt.Errorf("the setup's args hold %q, which the adapter does not give the mock", w)
		}
	}
	return pass, nil
}

// flagsOf are the run options the mock is given for the recorded run, as one string of words.
func flagsOf(setup string) string {
	pass, _ := runFlags(readFile(filepath.Join(setup, "args")))
	return strings.Join(pass, " ")
}

// sandboxMode is whether word is one of the sandbox modes Codex's -s names.
func sandboxMode(word string) bool {
	return word == "read-only" || word == "workspace-write" || word == "danger-full-access"
}
