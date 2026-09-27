// Package runner executes the user-supplied script, streams its JSONL output,
// validates each record, and fires Claude Code lifecycle hooks at the
// appropriate points.
//
// sr:docs https://docs.anthropic.com/en/docs/claude-code/hooks
// sr:docs https://docs.anthropic.com/en/docs/claude-code/cli-reference
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

// Config holds the runtime parameters for a mock run.
type Config struct {
	// ScriptPath is the shell script to execute. Its stdout is streamed as JSONL.
	ScriptPath string
	// SessionID is the Claude Code session identifier passed via --resume or --session-id.
	SessionID string
	// AgentID is the sub-agent's id when this Config drives a nested SUB-AGENT run (set by
	// runAgentTool). Empty for the ROOT run. It is stamped onto every PreToolUse payload
	// fired inside this run, so a hook can tell a sub-agent's tool call from the root's —
	// mirroring real claude, where a sub-agent's PreToolUse carries agent_id.
	AgentID string
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
	// sr:docs https://code.claude.com/docs/en/env-vars#environment-variables (CLAUDE_CODE_PLUGIN_CACHE_DIR)
	PluginCacheDir string
	// ConfigDir overrides the Claude Code global config directory (CLAUDE_CONFIG_DIR).
	// When empty, a temporary directory is created and cleaned up after the run.
	// The session JSONL is written under <ConfigDir>/projects/<encoded-cwd>/<session-id>.jsonl.
	// sr:docs https://code.claude.com/docs/en/agent-sdk/sessions (CLAUDE_CONFIG_DIR)
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
	// sr:docs https://code.claude.com/docs/en/hooks#subagentstart
	SuppressSubagentHooks bool

	// AgentType is the sub-agent's type when this Config drives a nested SUB-AGENT
	// run. Real Claude Code sends agent_type beside agent_id on every hook event
	// fired inside a sub-agent.
	// sr:docs https://code.claude.com/docs/en/hooks#common-input-fields
	AgentType string

	// SidechainPath is the sub-agent's own transcript
	// (<session>/subagents/agent-<id>.jsonl) for a nested SUB-AGENT run. Real
	// Claude Code writes a sub-agent's records there, never into the parent's
	// file: of 8,119 main transcripts on one machine, zero carry a sidechain
	// record. Empty for the ROOT run.
	SidechainPath string

	// ParentTranscriptPath is the transcript_path the PARENT session reports,
	// carried on every hook payload fired inside a nested sub-agent run — real
	// Claude Code's transcript_path is the session's, with the sub-agent named by
	// agent_id (and its own file by agent_transcript_path, on SubagentStop only).
	ParentTranscriptPath string

	// ForkFrom is the session id --resume named when --fork-session was given:
	// the run continues that conversation under SessionID, in a NEW transcript.
	// See forkTranscript for the shape it writes.
	// sr:docs https://code.claude.com/docs/en/cli-reference#--fork-session
	ForkFrom string

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
	// sr:docs https://code.claude.com/docs/en/cli-reference#--print
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

	// Resolve the config dir. Default is /tmp/a10n/claude-mock; overridable via
	// cfg.ConfigDir or CLAUDE_CONFIG_DIR env var (same variable the real Claude
	// Code CLI honours). Session files persist between turns so the script can
	// read history.
	// sr:docs https://code.claude.com/docs/en/agent-sdk/sessions (CLAUDE_CONFIG_DIR)
	// sr:docs https://code.claude.com/docs/en/claude-directory
	cfg.ConfigDir = resolveConfigDir(cfg.ConfigDir)

	// Which file this run writes, and which path its hooks are told about. See
	// transcript for why those differ and why a fresh session's file does not
	// exist yet when SessionStart fires.
	tr, err := openRunTranscript(cfg)
	if err != nil {
		return fmt.Errorf("claude-mock: open session file: %w", err)
	}
	defer tr.Close()

	nested := cfg.SuppressSubagentHooks

	settings, err := hooks.LoadSettings(cfg.ProjectDir, cfg.PluginCacheDir)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "claude-mock: warn: loading settings: %v\n", err)
		settings = &hooks.Settings{Hooks: make(map[hooks.EventName][]hooks.HookEntry)}
	}
	inv := hooks.NewInvoker(settings, cfg.Cwd, cfg.SessionID)
	inv.SetTranscriptPath(tr.reported)
	inv.SetRecorder(tr.recordHookRuns)
	if cfg.AgentID != "" {
		inv.SetAgent(cfg.AgentID, cfg.AgentType)
	}

	// SessionStart — fires once per top-level invocation (new, resumed or forked
	// session). Not for a nested sub-agent run: real Claude Code fires no
	// SessionStart for a sub-agent, which starts with SubagentStart instead.
	// The real payload uses "source" ("startup" | "resume" | "clear" |
	// "compact") — verified empirically against claude 2.x.
	// sr:docs https://code.claude.com/docs/en/hooks#sessionstart
	if !nested {
		source := "startup"
		if cfg.IsResume {
			source = "resume"
		}
		ac, err := fireSessionStart(ctx, cfg, inv, source)
		if err != nil {
			return fmt.Errorf("claude-mock: SessionStart hook blocked: %w", err)
		}
		// Real Claude Code injects a SessionStart hook's additionalContext into
		// the session context. Surface it so the script also sees it via env.
		if ac != "" {
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
	//   - RESUME: the NEXT human turn, chained into the transcript on disk — never
	//     a second parentless root.
	//   - NESTED sub-agent run: nothing here. The sub-agent's human-origin record
	//     is its dispatch prompt, seeded into its sidechain file by runAgentTool;
	//     writing it again would forge a human message the user never sent.
	switch {
	case nested:
	case cfg.IsResume:
		appendResumePrompt(tr, cfg.SessionID, cfg.Prompt)
	default:
		writeRootPrompt(tr, cfg.SessionID, cfg.Cwd, cfg.Prompt)
	}

	// In print mode: skip SubagentStart/SubagentStop/session-streaming; run the
	// script once with raw stdout capture, fire UserPromptSubmit + Stop, and return.
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
		runErr := runPrintMode(ctx, cfg, inv, tr) //nolint:contextcheck
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
		cfg.AdditionalContext = additionalContextFrom(promptOut)
	}

	// SubagentStart fires for resumed sessions (subagents always use --resume).
	// The agent_type defaults to "general-purpose"; scripts can override via a
	// subagent_start control record to signal a different agent type.
	// Suppressed for nested Agent-tool runs — see Config.SuppressSubagentHooks —
	// and for a fork, which is a new top-level session rather than a sub-agent.
	// sr:docs https://docs.anthropic.com/en/docs/claude-code/hooks#subagentstart
	if cfg.IsResume && !nested && cfg.ForkFrom == "" {
		if _, err := inv.Fire(ctx, hooks.Input{
			SessionID:     cfg.SessionID,
			Cwd:           cfg.Cwd,
			HookEventName: hooks.EventSubagentStart,
			AgentType:     "general-purpose",
		}); err != nil {
			return fmt.Errorf("claude-mock: SubagentStart hook blocked: %w", err)
		}
	}

	// streamAndHook owns the SUCCESS Stop lifecycle: on a clean end-of-turn it fires Stop,
	// surfaces a blocking Stop's output into the transcript, and re-prompts (loops) on a
	// block — mirroring real Claude Code's Stop→re-prompt→continue — until Stop is
	// non-blocking or the block cap is hit. So on success we do NOT fire Stop again here.
	// On a script ERROR streamAndHook returns early WITHOUT firing Stop, so we fire the
	// error-Stop here (stop_reason="error") — real Claude fires Stop regardless of outcome.
	runErr := streamAndHook(ctx, cfg, inv, tr)
	if runErr != nil && !nested {
		stopOut, stopErr := inv.Fire(ctx, hooks.Input{
			SessionID:     cfg.SessionID,
			Cwd:           cfg.Cwd,
			HookEventName: hooks.EventStop,
			StopReason:    "error",
		})
		_, _ = stopOut, stopErr // its attachment is written as it fires (transcript.recordHookRuns)
	}

	if !nested {
		// sr:docs https://docs.anthropic.com/en/docs/claude-code/hooks#sessionend
		_, _ = inv.Fire(ctx, hooks.Input{
			SessionID:     cfg.SessionID,
			Cwd:           cfg.Cwd,
			HookEventName: hooks.EventSessionEnd,
			Source:        "prompt_input_exit",
		})
	}

	return runErr
}

// stopBlockReason is the reason a Stop/SubagentStop fire blocked with, or "" if
// it did not block. An exit-2 block carries it on the error, an exit-0
// {"decision":"block"} on out.Reason.
func stopBlockReason(out hooks.Output, fireErr error) string {
	if fireErr != nil {
		return strings.TrimPrefix(fireErr.Error(), "hooks: command blocked: ")
	}
	if out.Decision == "block" {
		if out.Reason != "" {
			return out.Reason
		}
		return "blocked"
	}
	return ""
}

// runPrintMode runs the supervisor script once with raw stdout capture (no JSONL
// parsing, no session persistence). The script's working directory is cfg.Cwd
// (set to the session dir by the supervisor caller). Raw stdout is written to
// cfg.Out. This implements `claude --print` semantics for the autopilot supervisor.
//
// sr:docs https://code.claude.com/docs/en/cli-reference#--print
func runPrintMode(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript) error {
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

	// Opt-in: drive the script through the streaming turn loop so it can emit
	// tool_use records (e.g. an Agent/Task tool_use → nested sub-agent). The
	// supervisor still writes response.json as a side effect inside cfg.Cwd; the
	// streamed JSONL goes to cfg.Out as usual.
	if os.Getenv("A10N_MOCK_PRINT_STREAM") == "1" {
		return streamAndHook(ctx, cfg, inv, tr)
	}

	cmd := exec.CommandContext(ctx, "/bin/sh", cfg.ScriptPath) //nolint:gosec
	cmd.Dir = cfg.Cwd                                          // supervisor writes memory files here
	cmd.Env = buildEnv(cfg, tr)
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

// fireSessionStart fires the SessionStart hook with the given source field
// ("startup" | "resume" | "compact" | …), surfaces any additionalContext the hook
// returned (emitting a system record on the output stream, as real Claude Code
// injects a SessionStart hook's additionalContext into the session context — most
// notably on source="compact", to re-seed a compacted window), and returns that
// additionalContext. It is called once at startup/resume, and again on every
// compaction record the scenario emits (source="compact"; see scanLines). A
// blocking hook (exit 2) is returned as an error.
// sr:docs https://code.claude.com/docs/en/hooks#sessionstart
func fireSessionStart(ctx context.Context, cfg Config, inv *hooks.Invoker, source string) (string, error) {
	ssOut, err := inv.Fire(ctx, hooks.Input{
		SessionID:     cfg.SessionID,
		Cwd:           cfg.Cwd,
		HookEventName: hooks.EventSessionStart,
		Source:        source,
	})
	if err != nil {
		return "", err
	}
	ac := additionalContextFrom(ssOut)
	if ac != "" {
		emitSystemContext(cfg, "session_start", ac)
	}
	return ac, nil
}

// additionalContextFrom extracts the additionalContext a hook returned, if any.
// Real Claude Code appends this to the model's context; the mock forwards it to
// the script via A10N_MOCK_ADDITIONAL_CONTEXT.
func additionalContextFrom(out hooks.Output) string {
	if out.HookSpecificOutput != nil {
		return out.HookSpecificOutput.AdditionalContext
	}
	return ""
}

// emitSystemContext writes a JSONL system record carrying additionalContext to the
// output stream, mirroring how real Claude Code surfaces a hook's injected context
// (e.g. a SessionStart compact re-seed). source identifies the originating hook.
func emitSystemContext(cfg Config, source, additionalContext string) {
	rec := map[string]any{
		"type":              "system",
		"subtype":           "hook_additional_context",
		"source":            source,
		"additionalContext": additionalContext,
	}
	if b, err := json.Marshal(rec); err == nil {
		cfg.Out.Write(b)            //nolint:errcheck
		cfg.Out.Write([]byte{'\n'}) //nolint:errcheck
	}
}
