// Package runner executes the user-supplied script, streams its JSONL output,
// validates each record, and fires Claude Code lifecycle hooks at the
// appropriate points.
//
// a10n:docs https://docs.anthropic.com/en/docs/claude-code/hooks
// a10n:docs https://docs.anthropic.com/en/docs/claude-code/cli-reference
package runner

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/a10n-build/a10n-cli/services/claude-mock/internal/hooks"
)

// Config holds the runtime parameters for a mock run.
type Config struct {
	// ScriptPath is the shell script to execute. Its stdout is streamed as JSONL.
	ScriptPath string
	// SessionID is the Claude Code session identifier passed via --resume or --session-id.
	SessionID string
	// IsResume is true when the caller used --resume (existing session) vs --session-id (new).
	IsResume bool
	// Prompt is the user prompt forwarded to the script via the A10N_MOCK_PROMPT env var.
	Prompt string
	// Cwd is the working directory for the script and hook invocations.
	Cwd string
	// ProjectDir is used to resolve .claude/settings.json for hook configuration.
	ProjectDir string
	// ConfigDir overrides the Claude Code global config directory (CLAUDE_CONFIG_DIR).
	// When empty, a temporary directory is created and cleaned up after the run.
	// The session JSONL is written under <ConfigDir>/projects/<encoded-cwd>/<session-id>.jsonl.
	// a10n:docs https://code.claude.com/docs/en/agent-sdk/sessions (CLAUDE_CONFIG_DIR)
	ConfigDir string
	// Stderr receives diagnostic output from the mock itself.
	Stderr io.Writer
	// Out receives the passthrough JSONL (defaults to os.Stdout).
	Out io.Writer
}

// Run executes the mock: runs the script, validates + streams JSONL, fires hooks.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Out == nil {
		cfg.Out = os.Stdout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	if cfg.ScriptPath == "" {
		return fmt.Errorf("claude-mock: --script is required")
	}

	// Resolve the config dir and open the session JSONL.
	// Default is /tmp/a10n/claude-mock; overridable via cfg.ConfigDir or
	// CLAUDE_CONFIG_DIR env var (same variable the real Claude Code CLI honours).
	// Session files persist between turns so the script can read history.
	// a10n:docs https://code.claude.com/docs/en/agent-sdk/sessions (CLAUDE_CONFIG_DIR)
	// a10n:docs https://code.claude.com/docs/en/claude-directory
	cfg.ConfigDir = resolveConfigDir(cfg.ConfigDir)

	sessionFile, err := openSessionFile(cfg.ConfigDir, cfg.Cwd, cfg.SessionID)
	if err != nil {
		return fmt.Errorf("claude-mock: open session file: %w", err)
	}
	defer sessionFile.Close()

	settings, err := hooks.LoadSettings(cfg.ProjectDir)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "claude-mock: warn: loading settings: %v\n", err)
		settings = &hooks.Settings{Hooks: make(map[hooks.EventName][]hooks.HookEntry)}
	}
	inv := hooks.NewInvoker(settings, cfg.Cwd)

	// SessionStart hook — fires for every invocation (new or resumed session).
	// a10n:docs https://docs.anthropic.com/en/docs/claude-code/hooks#session-start
	source := "startup"
	if cfg.IsResume {
		source = "resume"
	}
	if _, err := inv.Fire(ctx, hooks.Input{
		SessionID:     cfg.SessionID,
		Cwd:           cfg.Cwd,
		HookEventName: hooks.EventSessionStart,
		Source:        source,
	}); err != nil {
		return fmt.Errorf("claude-mock: SessionStart hook blocked: %w", err)
	}

	// SubagentStart fires for resumed sessions (subagents always use --resume).
	// The agent_type defaults to "general-purpose"; scripts can override via a
	// subagent_start control record to signal a different agent type.
	// a10n:docs https://docs.anthropic.com/en/docs/claude-code/hooks#subagentstart
	if cfg.IsResume {
		if _, err := inv.Fire(ctx, hooks.Input{
			SessionID:     cfg.SessionID,
			Cwd:           cfg.Cwd,
			HookEventName: hooks.EventSubagentStart,
			AgentType:     "general-purpose",
		}); err != nil {
			return fmt.Errorf("claude-mock: SubagentStart hook blocked: %w", err)
		}
	}

	runErr := streamAndHook(ctx, cfg, inv, sessionFile)

	// Fire Stop + SessionEnd regardless of script exit status.
	// a10n:docs https://docs.anthropic.com/en/docs/claude-code/hooks#stop
	stopReason := "end_turn"
	if runErr != nil {
		stopReason = "error"
	}
	_, _ = inv.Fire(ctx, hooks.Input{
		SessionID:     cfg.SessionID,
		Cwd:           cfg.Cwd,
		HookEventName: hooks.EventStop,
		StopReason:    stopReason,
	})
	_, _ = inv.Fire(ctx, hooks.Input{
		SessionID:     cfg.SessionID,
		Cwd:           cfg.Cwd,
		HookEventName: hooks.EventSessionEnd,
		Source:        "prompt_input_exit",
	})

	return runErr
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
