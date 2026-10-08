package replay

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// cmdFlags are the flags the recorded `codex exec` command line had that the mock takes with the same
// name; -m is the model's name (the mock's is its own), and the command line holds nothing else the
// adapter can give it. A run made with a flag the adapter does not know is not replayed.
func cmdFlags(command string) (pass []string, err error) {
	words := strings.Fields(command)
	if len(words) < 2 || words[0] != "codex" || words[1] != "exec" {
		return nil, fmt.Errorf("recorded with %q, not a codex exec command line", command)
	}
	for i := 2; i < len(words); i++ {
		switch w := words[i]; w {
		case "--json", "--skip-git-repo-check", "--dangerously-bypass-approvals-and-sandbox", "--dangerously-bypass-hook-trust":
			pass = append(pass, w)
		case "-m":
			i++
		default:
			return nil, fmt.Errorf("recorded with %s, which the adapter does not give the mock", w)
		}
	}
	return pass, nil
}

// refusedRun is a recording of a run that never started a session: the harness refused it (no rollout, no
// events, a non-zero exit). What is replayed is the refusal: the mock must exit as the harness did, with
// nothing in its event stream.
func refusedRun(runDir, setup string, setupMap map[string]string) core.Recording {
	return core.Recording{Dir: runDir, Prompt: strings.TrimSpace(readFile(filepath.Join(setup, "prompt.txt"))), Setup: setupMap}
}

// setupOf is what the replay is given of a recorded run's setup, by name: its hooks and hook script, the
// options of its command line and of its args, whether it ran outside a repository, and how it ended.
func setupOf(setup, sample string, cmdline []string) map[string]string {
	_, noGit := os.Stat(filepath.Join(setup, "no-git"))
	return map[string]string{
		"hooks.json":         readFile(filepath.Join(setup, "hooks.json")),
		"hook.sh":            readFile(filepath.Join(setup, "hook.sh")),
		"project-hooks.json": readFile(filepath.Join(setup, "project-hooks.json")),
		"flags":              flagsOf(setup),
		"cmdflags":           strings.Join(cmdline, " "),
		"no-git":             fmt.Sprint(noGit == nil),
		"exit":               exitOf(sample),
		"env":                strings.TrimSpace(readFile(filepath.Join(setup, "env"))),
		"prepare.sh":         readFile(filepath.Join(setup, "prepare.sh")),
		"trust-hooks":        readFile(filepath.Join(setup, "trust-hooks")),
		"hooks-list.json":    readFile(filepath.Join(sample, "hooks-list.json")),
	}
}

// exitOf is the exit status the recorded first run ended with.
func exitOf(sample string) string {
	b, _ := os.ReadFile(filepath.Join(sample, "exit.txt"))
	return strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
}
