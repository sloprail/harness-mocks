package main

import "os"

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
