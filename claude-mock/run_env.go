package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// hasScratchpad is whether the session has a scratchpad directory. Recorded: a run whose
// CLAUDE_CODE_ENTRYPOINT is a value claude does not know has one, and its hooks are told of it
// (scratchpad_dir); under the entrypoints claude -p takes by itself (sdk-cli, cli) it has none
// (snapshots/runs/scratchpad-dir, nested-session-env). Read once here, with the rest of the configuration.
// sr:docs https://code.claude.com/docs/en/hooks#common-input-fields
func hasScratchpad() bool {
	switch os.Getenv("CLAUDE_CODE_ENTRYPOINT") {
	case "", "sdk-cli", "cli":
		return false
	}
	return true
}

// stdinGiven is whether stdin is a pipe or a file with content rather than a terminal or /dev/null.
func stdinGiven() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && (fi.Mode()&os.ModeNamedPipe != 0 || fi.Mode().IsRegular() && fi.Size() > 0)
}

// stdinPrompt is the prompt `claude -p` reads from a piped stdin when it is given no prompt
// argument, minus the trailing newline of the pipe.
// sr:docs https://code.claude.com/docs/en/headless#basic-usage
func stdinPrompt() (string, error) {
	if !stdinGiven() {
		return "", nil
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("claude-mock: read stdin: %w", err)
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}

// promptFrom is the run's prompt: the positional arguments, or, with none, the piped stdin.
func promptFrom(args []string) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}
	return stdinPrompt()
}
