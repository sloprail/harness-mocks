package runner

import "github.com/sloprail/harness-mocks/claude-mock/internal/hooks"

// writeHookEventFrames streams the frames of a hook a main-thread event fired
// when --include-hook-events asks for them (recorded: snapshots/runs/include-hook-events:
// UserPromptSubmit, PreToolUse:<tool>, PostToolUse:<tool> and Stop each stream a
// started and a response frame, in firing order). SessionStart's are always streamed.
func writeHookEventFrames(cfg Config, in hooks.Input, runs []hooks.HandlerRun) {
	if cfg.HookEvents && cfg.AgentID == "" {
		writeSessionStartFrames(cfg, in, runs)
	}
}
