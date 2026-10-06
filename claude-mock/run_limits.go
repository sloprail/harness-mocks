package main

import (
	"os"
	"strconv"
	"time"

	"github.com/sloprail/harness-mocks/internal/tasks"
)

// printWaitCeiling is the ceiling on a `claude -p` run's idle wait for background
// agents: CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS in milliseconds (0 waits without
// one), else ten minutes. Read once here, with the rest of the configuration.
// sr:docs https://code.claude.com/docs/en/env-vars#environment-variables
func printWaitCeiling() time.Duration {
	if ms, err := strconv.Atoi(os.Getenv("CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS")); err == nil && ms >= 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return tasks.DefaultWaitCeiling
}

// spawnLimit is how many layers of sub-agents nest below the main conversation:
// CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH, else 0 for the default of three. Read
// once here, with the rest of the configuration.
// sr:docs https://code.claude.com/docs/en/sub-agents#let-subagents-spawn-their-own-subagents
func spawnLimit() int {
	if n, err := strconv.Atoi(os.Getenv("CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH")); err == nil && n > 0 {
		return n
	}
	return 0
}

// concurrentLimit is how many sub-agents may run at once: CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS when it is
// a positive number, else 0 for the default. Read once here, with the rest of the configuration.
// (recorded: runs/bgagent-concurrent-limit)
func concurrentLimit() int {
	if n, err := strconv.Atoi(os.Getenv("CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS")); err == nil && n > 0 {
		return n
	}
	return 0
}

// backgroundTasksDisabled is whether CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1
// turns the background task functionality off. Read once here, with the rest
// of the configuration.
// sr:docs https://code.claude.com/docs/en/tools-reference#background-commands
// sr:provides background-bash/claude
func backgroundTasksDisabled() bool {
	return os.Getenv("CLAUDE_CODE_DISABLE_BACKGROUND_TASKS") == "1"
}
