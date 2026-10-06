package replay

import (
	"path/filepath"
)

func hasFlag(flags []string, flag string) bool {
	for _, f := range flags {
		if f == flag {
			return true
		}
	}
	return false
}

// eventStream is what a recorded run printed on stdout, as objects: JSON events for a run made with
// --json, and for one made without it (stdout is only the agent's last message) its lines, each as
// {"raw": line}.
func eventStream(sample string, cmdline []string) ([]map[string]any, error) {
	if hasFlag(cmdline, "--json") {
		return readJSONL(filepath.Join(sample, "stream.jsonl"))
	}
	return parseHookLog(readFile(filepath.Join(sample, "stream.jsonl")))
}

// parseStream is what the mock printed on stdout, as eventStream reads a recorded run's.
func parseStream(stdout string, cmdline []string) ([]map[string]any, error) {
	if hasFlag(cmdline, "--json") {
		return parseJSONL(stdout)
	}
	return parseHookLog(stdout)
}
