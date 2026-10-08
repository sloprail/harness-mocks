package runner

import "strings"

// isLocalCommand is whether a prompt is the /compact command, which the harness carries out itself:
// no UserPromptSubmit hook sees it (recorded: runs/compact, compact-nohooks).
func isLocalCommand(prompt string) bool {
	return strings.Fields(prompt + " ")[0] == "/compact"
}
