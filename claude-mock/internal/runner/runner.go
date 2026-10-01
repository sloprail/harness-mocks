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

	settings, err := hooks.LoadSettings(cfg.ProjectDir, cfg.PluginCacheDir)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "claude-mock: warn: loading settings: %v\n", err)
		settings = &hooks.Settings{Hooks: make(map[hooks.EventName][]hooks.HookEntry)}
	}

	// --resume (with or without --fork-session) of a session no transcript
	// holds: real Claude Code fires no SessionStart, fires SessionEnd, prints
	// "No conversation found with session ID: <id>" and exits 1.
	if !nested && cfg.IsResume {
		from := cfg.SessionID
		if cfg.ForkFrom != "" {
			from = cfg.ForkFrom
		}
		if sessionFilePathIfExists(cfg.ConfigDir, cfg.Cwd, from) == "" {
			inv := hooks.NewInvoker(settings, cfg.Cwd, from)
			inv.SetTranscriptPath(sessionFilePath(cfg.ConfigDir, cfg.Cwd, from))
			fireSessionEnd(ctx, cfg, inv)
			return &ErrNoConversation{SessionID: from}
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

	inv := hooks.NewInvoker(settings, cfg.Cwd, cfg.SessionID)
	inv.SetTranscriptPath(tr.reported)
	inv.SetRecorder(tr.recordHookRuns)
	if cfg.AgentID != "" {
		inv.SetAgent(cfg.AgentID, cfg.AgentType)
	}

	// SessionStart — once per top-level invocation. Not for a nested sub-agent
	// run: real Claude Code fires no SessionStart for a sub-agent, which starts
	// with SubagentStart instead. source is "startup", "resume", or "fork" for
	// `--resume <id> --fork-session` (claude 2.1.282; docs). An exit 2 does not
	// stop the session: real Claude Code records it as a non-blocking error
	// and carries on (docs, "Exit code 2 behavior per event").
	// sr:docs https://code.claude.com/docs/en/hooks#sessionstart
	if !nested {
		source := "startup"
		switch {
		case cfg.ForkFrom != "":
			source = "fork"
		case cfg.IsResume:
			source = "resume"
		}
		// Real Claude Code injects a SessionStart hook's additionalContext into
		// the session context. Surface it so the script also sees it via env.
		if ac := fireSessionStart(ctx, cfg, inv, source); ac != "" {
			cfg.AdditionalContext = ac
		}
	}

	// The prompt, as the HUMAN turn it is — AFTER SessionStart, as real Claude
	// Code writes it:
	//
	//   - FRESH: the file may not exist yet. If SessionStart printed anything its
	//     attachment is already the file's origin and the prompt chains after it;
	//     if not, the prompt is the first record and so the origin itself. Its
	//     uuid is the deterministic `e2e-root-<session>` either way, so a caller
	//     can reference the human message up front.
	//   - RESUME / FORK: the NEXT human turn, chained into the transcript on
	//     disk — never a second parentless root.
	//   - NESTED sub-agent run: nothing here. The sub-agent's human-origin record
	//     is its dispatch prompt, seeded into its sidechain file by
	//     prepareSubagent; writing it again would forge a human message the user
	//     never sent.
	switch {
	case nested:
	case cfg.IsResume:
		appendResumePrompt(tr, cfg.SessionID, cfg.Prompt)
	default:
		writeRootPrompt(tr, cfg.SessionID, cfg.Cwd, cfg.Prompt)
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
	if cfg.Prompt != "" && !nested {
		promptOut, err := inv.Fire(ctx, hooks.Input{
			SessionID:     cfg.SessionID,
			Cwd:           cfg.Cwd,
			HookEventName: hooks.EventUserPromptSubmit,
			Prompt:        cfg.Prompt,
		})
		if err != nil {
			return fmt.Errorf("claude-mock: UserPromptSubmit hook blocked: %w", err)
		}
		cfg.AdditionalContext = addContext(cfg.AdditionalContext, promptContextFrom(promptOut))
	}

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
