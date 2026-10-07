package hooks

import (
	"strings"
	"testing"
)

// The hashes are the ones Codex 0.159.3 reported for the same hook definitions (app-server hooks/list).
func TestHookHashIsCodexs(t *testing.T) {
	cmd := func(c string) fileHandler { return fileHandler{Type: "command", Command: c} }
	for _, tc := range []struct {
		name    string
		ev      Event
		matcher string
		h       fileHandler
		want    string // the hash, or the start of it
	}{
		{"project", PreToolUse, "Bash", cmd(`"$(git rev-parse --show-toplevel)"/hook.sh project`), "sha256:fe1d1399af4be296a812de676629165e585a6b201058fb71959eb1bd91cc5d4d"},
		{"user", PreToolUse, "Bash", cmd(`"$(git rev-parse --show-toplevel)"/hook.sh user`), "sha256:d978dfb3851e6eb6af88fd8323e578998edac8eb593f6b962c2ede394cbe6b41"},
		{"plugin", PreToolUse, "Bash", cmd(`env | grep -E 'PLUGIN|CODEX|CLAUDE' >> "$HOOK_LOG"`), "sha256:31e22a0a4d1563e8c867d23b50f72112af057bfbe14541f82840fbaeb773acdf"},
		{"timeout", PreToolUse, "Bash", fileHandler{Type: "command", Command: "echo b", Timeout: 5}, "sha256:cb0d3f90"},
		{"status message", PreToolUse, "Bash", fileHandler{Type: "command", Command: "echo c", StatusMessage: "hi"}, "sha256:22f0e12a"},
		{"no matcher", SessionStart, "", cmd("echo a"), "sha256:c79c3cd4"},
		{"async, and a matcher the event ignores", Stop, "x", fileHandler{Type: "command", Command: "echo d", Async: true}, "sha256:a1bbdbad"},
	} {
		if got := hookHash(tc.ev, tc.matcher, tc.h); !strings.HasPrefix(got, tc.want) {
			t.Errorf("%s: hash %s, Codex's %s", tc.name, got, tc.want)
		}
	}
}
