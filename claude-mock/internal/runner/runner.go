// Package runner executes the user-supplied script, streams its JSONL output,
// validates each record, and fires Claude Code lifecycle hooks at the
// appropriate points.
//
// sr:docs https://docs.anthropic.com/en/docs/claude-code/hooks
// sr:docs https://docs.anthropic.com/en/docs/claude-code/cli-reference
package runner

import (
	"context"
	"fmt"
	"os"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

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

	// Resolve the config dir. Default is /tmp/a10n/claude-mock; overridable via
	// cfg.ConfigDir or CLAUDE_CONFIG_DIR env var (same variable the real Claude
	// Code CLI honours). Session files persist between turns so the script can
	// read history.
	// sr:docs https://code.claude.com/docs/en/agent-sdk/sessions (CLAUDE_CONFIG_DIR)
	// sr:docs https://code.claude.com/docs/en/claude-directory
	cfg.ConfigDir = resolveConfigDir(cfg.ConfigDir)
	if cfg.stream == nil {
		lw := &lockedWriter{w: cfg.Out}
		cfg.Out, cfg.stream = lw, lw
	}

	nested := cfg.SuppressSubagentHooks

	settings, err := loadRunSettings(cfg)
	if err != nil {
		return err
	}

	// --resume (with or without --fork-session) of a session no transcript
	// holds: real Claude Code fires no SessionStart, fires SessionEnd, prints
	// "No conversation found with session ID: <id>" and exits 1.
	if !nested && cfg.IsResume {
		from := cfg.SessionID
		if cfg.ForkFrom != "" {
			from = cfg.ForkFrom
		}
		if err := resumeFailed(ctx, cfg, settings, from); err != nil {
			return err
		}
	}

	// Which file this run writes, and which path its hooks are told about. See
	// transcript for why those differ and why a fresh session's file does not
	// exist yet when SessionStart fires.
	tr, err := openRunTranscript(cfg)
	if err != nil {
		return fmt.Errorf("claude-mock: open session file: %w", err)
	}
	defer tr.Close()
	if cfg.sessionFile == "" {
		cfg.sessionFile = tr.path
	}

	var inv *hooks.Invoker
	inv, cfg.Turn = newInvoker(cfg, settings, tr)

	// SessionStart — once per top-level invocation. Not for a nested sub-agent
	// run: real Claude Code fires no SessionStart for a sub-agent, which starts
	// with SubagentStart instead. source is "startup", "resume", or "fork" for
	// `--resume <id> --fork-session` (claude 2.1.282; docs). An exit 2 does not
	// stop the session: real Claude Code records it as a non-blocking error
	// and carries on (docs, "Exit code 2 behavior per event").
	// sr:docs https://code.claude.com/docs/en/hooks#sessionstart
	if !nested {
		// Real Claude Code injects a SessionStart hook's additionalContext into
		// the session context. Surface it so the script also sees it via env.
		ac, err := fireSessionStart(ctx, cfg, inv, corehooks.SessionStartKind(cfg.IsResume, cfg.ForkFrom != "", false))
		if err != nil {
			return err
		}
		if ac != "" {
			cfg.AdditionalContext = ac
		}
	}

	// In print mode: run the script once with raw stdout capture, fire
	// UserPromptSubmit + Stop, and return.
	//
	// EXCEPTION (A10N_MOCK_PRINT_STREAM=1): process the script's stdout through the
	// normal streaming turn loop instead of raw passthrough, so a print-mode agent can
	// emit tool_use records — notably an Agent/Task tool_use that spawns a nested
	// sub-agent. The real `claude --print` is fully tool-capable (—print only means
	// non-interactive output); this opt-in lets the autopilot course-correction
	// supervisor (which delegates trajectory-slice review to a sub-agent) be exercised
	// faithfully in e2e. Default-off keeps every existing print-mode scenario (which
	// writes files / emits raw text) on the original raw path.
	// sr:docs https://code.claude.com/docs/en/cli-reference#--print
	if cfg.PrintMode {
		writePrompt(tr, cfg, nested)
		err := runPrintMode(ctx, cfg, inv, tr) //nolint:contextcheck
		fireSessionEnd(ctx, cfg, inv)
		return err
	}

	// UserPromptSubmit fires before the prompt reaches the model. Per the real
	// Claude Code contract the hook CANNOT replace the prompt — it may only append
	// additionalContext (exposed to the script via A10N_MOCK_ADDITIONAL_CONTEXT)
	// or block the prompt (decision=block / exit 2 → Fire returns an error). Not
	// for a nested sub-agent run: the sub-agent's prompt is its dispatcher's
	// tool input, not something a user submitted.
	// sr:docs https://code.claude.com/docs/en/hooks#userpromptsubmit
	src := corehooks.PromptFromUser
	if nested {
		src = corehooks.PromptSubagentDispatch
	} else if isLocalCommand(cfg.Prompt) {
		src = corehooks.PromptLocalCommand
	}
	extra, refused, err := submitPrompt(ctx, cfg, inv, tr, src, true)
	if refused {
		return err
	}
	cfg.AdditionalContext = corehooks.JoinContext(cfg.AdditionalContext, extra)
	// The prompt is written once UserPromptSubmit has let it through, and what that
	// hook left follows it (see writePrompt).
	writePrompt(tr, cfg, nested)
	tr.flushHookRuns()

	// streamAndHook owns the turn lifecycle: Stop at every end of turn, the
	// re-prompt on a block, and the turns background work starts after it. A
	// script that fails ends the run without a Stop: real Claude Code fires
	// Stop only when the agent finishes responding (API errors fire
	// StopFailure, which the mock does not model).
	// sr:docs https://code.claude.com/docs/en/hooks#stop
	runErr := streamAndHook(ctx, cfg, inv, tr)

	if !nested {
		fireSessionEnd(ctx, cfg, inv)
	}

	return runErr
}
