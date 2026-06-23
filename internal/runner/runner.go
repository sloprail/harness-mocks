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
	"os/exec"

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
	// AdditionalContext is populated from a UserPromptSubmit hook's additionalContext
	// output and forwarded to the script via A10N_MOCK_ADDITIONAL_CONTEXT. This is the
	// real Claude Code mechanism by which a UserPromptSubmit hook augments (never
	// replaces) what the model sees.
	AdditionalContext string
	// Cwd is the working directory for the script and hook invocations.
	Cwd string
	// ProjectDir is used to resolve .claude/settings.json for hook configuration.
	ProjectDir string
	// PluginCacheDir overrides the Claude Code plugin cache root (CLAUDE_CODE_PLUGIN_CACHE_DIR).
	// Marketplaces are cloned/reused under <PluginCacheDir>/<marketplace-slug>/.
	// When empty, falls back to the env var, then /tmp/a10n-mock-plugins.
	// a10n:docs https://code.claude.com/docs/en/env-vars#environment-variables (CLAUDE_CODE_PLUGIN_CACHE_DIR)
	PluginCacheDir string
	// ConfigDir overrides the Claude Code global config directory (CLAUDE_CONFIG_DIR).
	// When empty, a temporary directory is created and cleaned up after the run.
	// The session JSONL is written under <ConfigDir>/projects/<encoded-cwd>/<session-id>.jsonl.
	// a10n:docs https://code.claude.com/docs/en/agent-sdk/sessions (CLAUDE_CONFIG_DIR)
	ConfigDir string
	// Stderr receives diagnostic output from the mock itself.
	Stderr io.Writer
	// Out receives the passthrough JSONL (defaults to os.Stdout).
	Out io.Writer

	// SuppressSubagentHooks disables this run's own SubagentStart (on resume) and
	// SubagentStop (on end_turn) firing. It is set ONLY for nested subagent runs
	// spawned via the Agent (alias Task) tool: the Agent-tool layer fires
	// SubagentStart/SubagentStop itself, WITH the generated agent_id, so the nested
	// run must not double-fire those events with an agent_id-less payload.
	// a10n:docs https://code.claude.com/docs/en/hooks#subagentstart
	SuppressSubagentHooks bool

	// PrintMode activates --print mode: the script runs in cfg.Cwd as its working
	// directory, its raw stdout is captured (no JSONL parsing, no session
	// persistence), and written to cfg.Out. Only SessionStart, UserPromptSubmit,
	// and Stop hooks fire. This mirrors the real `claude --print` non-interactive
	// mode used by the autopilot supervisor.
	//
	// The supervisor (RunSupervisor in services/task-executor) invokes claude with
	// --print and expects:
	//  - cmd.Dir = session dir (the script writes memory files there)
	//  - stdout = raw text (the unclassified user prompt remainder), not JSONL
	//  - no session JSONL written
	//  - SessionStart/UserPromptSubmit/Stop hooks still fire so plugins can intercept
	//
	// a10n:docs https://code.claude.com/docs/en/cli-reference#--print
	PrintMode bool
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

	settings, err := hooks.LoadSettings(cfg.ProjectDir, cfg.PluginCacheDir)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "claude-mock: warn: loading settings: %v\n", err)
		settings = &hooks.Settings{Hooks: make(map[hooks.EventName][]hooks.HookEntry)}
	}
	inv := hooks.NewInvoker(settings, cfg.Cwd, cfg.SessionID)

	// SessionStart hook — fires for every invocation (new or resumed session).
	// The real Claude Code payload uses the "trigger" field ("startup" | "resume" |
	// "clear" | "compact"); "source" is a separate field used by SessionEnd.
	// a10n:docs https://code.claude.com/docs/en/hooks#sessionstart
	trigger := "startup"
	if cfg.IsResume {
		trigger = "resume"
	}
	if _, err := inv.Fire(ctx, hooks.Input{
		SessionID:     cfg.SessionID,
		Cwd:           cfg.Cwd,
		HookEventName: hooks.EventSessionStart,
		Trigger:       trigger,
	}); err != nil {
		return fmt.Errorf("claude-mock: SessionStart hook blocked: %w", err)
	}

	// In print mode: skip SubagentStart/SubagentStop/session-streaming; run the
	// script once with raw stdout capture, fire UserPromptSubmit + Stop, and return.
	// a10n:docs https://code.claude.com/docs/en/cli-reference#--print
	if cfg.PrintMode {
		runErr := runPrintMode(ctx, cfg, inv, sessionFile) //nolint:contextcheck
		stopReason := "end_turn"
		if runErr != nil {
			stopReason = "error"
		}
		// In print mode the Stop hook validates response.json (exit 2 = validation
		// failure). Propagate this as an error so RunSupervisor knows the run failed.
		_, stopErr := inv.Fire(ctx, hooks.Input{
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
		if runErr != nil {
			return runErr
		}
		return stopErr
	}

	// UserPromptSubmit fires before the prompt reaches the model. Per the real
	// Claude Code contract the hook CANNOT replace the prompt — it may only append
	// additionalContext (exposed to the script via A10N_MOCK_ADDITIONAL_CONTEXT)
	// or block the prompt (decision=block / exit 2 → Fire returns an error).
	// a10n:docs https://code.claude.com/docs/en/hooks#userpromptsubmit
	if cfg.Prompt != "" {
		promptOut, err := inv.Fire(ctx, hooks.Input{
			SessionID:     cfg.SessionID,
			Cwd:           cfg.Cwd,
			HookEventName: hooks.EventUserPromptSubmit,
			Prompt:        cfg.Prompt,
		})
		if err != nil {
			return fmt.Errorf("claude-mock: UserPromptSubmit hook blocked: %w", err)
		}
		cfg.AdditionalContext = additionalContextFrom(promptOut)
	}

	// SubagentStart fires for resumed sessions (subagents always use --resume).
	// The agent_type defaults to "general-purpose"; scripts can override via a
	// subagent_start control record to signal a different agent type.
	// Suppressed for nested Agent-tool runs — see Config.SuppressSubagentHooks.
	// a10n:docs https://docs.anthropic.com/en/docs/claude-code/hooks#subagentstart
	if cfg.IsResume && !cfg.SuppressSubagentHooks {
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

// runPrintMode runs the supervisor script once with raw stdout capture (no JSONL
// parsing, no session persistence). The script's working directory is cfg.Cwd
// (set to the session dir by the supervisor caller). Raw stdout is written to
// cfg.Out. This implements `claude --print` semantics for the autopilot supervisor.
//
// a10n:docs https://code.claude.com/docs/en/cli-reference#--print
func runPrintMode(ctx context.Context, cfg Config, inv *hooks.Invoker, sessionFile *os.File) error {
	// Fire UserPromptSubmit so any hook in the project-dir settings can intercept
	// even in print mode. The hook cannot replace the prompt (real Claude
	// contract); it may only append additionalContext or block (→ Fire errors).
	if cfg.Prompt != "" {
		promptOut, err := inv.Fire(ctx, hooks.Input{
			SessionID:     cfg.SessionID,
			Cwd:           cfg.Cwd,
			HookEventName: hooks.EventUserPromptSubmit,
			Prompt:        cfg.Prompt,
		})
		if err != nil {
			return fmt.Errorf("claude-mock: UserPromptSubmit hook blocked: %w", err)
		}
		cfg.AdditionalContext = additionalContextFrom(promptOut)
	}

	cmd := exec.CommandContext(ctx, "/bin/sh", cfg.ScriptPath) //nolint:gosec
	cmd.Dir = cfg.Cwd                                          // supervisor writes memory files here
	cmd.Env = buildEnv(cfg, sessionFile)
	cmd.Stderr = cfg.Stderr
	cmd.Stdout = cfg.Out // raw text passthrough — no JSONL parsing
	return cmd.Run()
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// additionalContextFrom extracts the additionalContext a UserPromptSubmit hook
// returned, if any. Real Claude Code appends this to the model's context; the
// mock forwards it to the script via A10N_MOCK_ADDITIONAL_CONTEXT.
func additionalContextFrom(out hooks.Output) string {
	if out.HookSpecificOutput != nil {
		return out.HookSpecificOutput.AdditionalContext
	}
	return ""
}
