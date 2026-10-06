package toolexec

import "strings"

// silentCommands are the commands whose success prints nothing, so that the structured result of a Bash
// call to one says noOutputExpected. Recorded: a `touch` says so (snapshots/runs/isolated-worktree), where
// `true`, a `sleep` and an `echo` of nothing do not. No other command is recorded, so no other is listed.
var silentCommands = map[string]bool{"touch": true}

// silentCommand is whether a Bash command is one simple call of a silent command.
func silentCommand(command string) bool {
	words := strings.Fields(command)
	return len(words) > 0 && silentCommands[words[0]] && !strings.ContainsAny(command, ";&|<>`$(\n")
}
