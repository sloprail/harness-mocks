package main

import (
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

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

// addRunFlags registers all flags needed to mimic the claude CLI interface.
// a10n:blueprint:ignore
func addRunFlags(cmd *cobra.Command) {
	cmd.Flags().String(flagScript, "", "Shell script to run as the mock agent (env: A10N_MOCK_SCRIPT)")
	cmd.Flags().String(flagSessionID, "", "Session ID (--session-id, as used by claude CLI)")
	cmd.Flags().StringP(flagResume, "r", "", "Session ID to resume (--resume, -r, as used by claude CLI)")
	// --continue (-c) resumes the most recent session of the directory (headless#continue-conversations);
	// --resume has the short form -r (cli-reference#cli-flags).
	cmd.Flags().BoolP(flagContinue, "c", false, "Resume the most recent session of the project directory (--continue, -c, as used by claude CLI)")
	cmd.Flags().Bool(flagNoPersistence, false, "Leave no session to resume (--no-session-persistence, as used by claude CLI)")
	// --fork-session: when resuming, continue under a NEW session id in a new
	// transcript instead of appending to the original. The new id is
	// --session-id when given, else generated. See runner.forkTranscript for the
	// transcript shape a fork leaves.
	// sr:docs https://code.claude.com/docs/en/cli-reference#--fork-session
	cmd.Flags().Bool(flagForkSession, false, "With --resume: continue in a new session id and transcript")
	cmd.Flags().String(flagOutputFormat, "stream-json", "Output format (must be stream-json)")
	cmd.Flags().String(flagProjectDir, "", "Project root for settings.json resolution (default: cwd)")
	cmd.Flags().String(flagConfigDir, "", "Claude config dir for session JSONL storage (env: CLAUDE_CONFIG_DIR, default: /tmp/a10n/claude-mock)")
	cmd.Flags().String(flagPluginCacheDir, "", "Plugin/marketplace cache root (env: CLAUDE_CODE_PLUGIN_CACHE_DIR, default: /tmp/a10n-mock-plugins)")
	// --print activates non-interactive print mode: the script's raw stdout is
	// forwarded directly (no JSONL parsing, no session persistence).
	// This mirrors `claude --print` used by the autopilot supervisor.
	// sr:docs https://code.claude.com/docs/en/cli-reference#--print
	cmd.Flags().Bool(flagPrint, false, "Print mode: forward raw script stdout instead of streaming JSONL")
	// -p is accepted for CLI compatibility with `claude -p` shorthand; it is ignored
	// (the --print flag above is the real mechanism). Existing tests use -p as a
	// "pass a prompt" shorthand that cobra treats as the bool flag being set, with
	// the following arg becoming a positional prompt — this must remain ignored.
	cmd.Flags().BoolP("print-compat", "p", false, "Accepted for CLI compatibility with -p shorthand; has no effect")
	_ = cmd.Flags().MarkHidden("print-compat")
	// claude also passes --verbose; accept but ignore.
	cmd.Flags().Bool("verbose", false, "Accepted for CLI compatibility; has no effect")
	// The autopilot supervisor passes these flags; accept them for CLI compatibility.
	cmd.Flags().String("system-prompt", "", "Accepted for CLI compatibility; passed to script via A10N_MOCK_SYSTEM_PROMPT")
	cmd.Flags().StringArray("add-dir", nil, "Accepted for CLI compatibility; has no effect")
	cmd.Flags().Bool("dangerously-skip-permissions", false, "Accepted for CLI compatibility; has no effect")

	// --- sr-agent (sloprail guardrail JUDGE) compatibility flags ---
	//
	// Downstream, the sloprail engine's guardrail judge runs `sr-agent`, which
	// invokes `claude` as a subprocess (services/sr-agent BuildInvocation):
	//
	//   claude -p --model <m> --add-dir <dir> --allowed-tools Write \
	//          [--permission-mode … --settings … --append-system-prompt …] -- <prompt>
	//
	// In e2e, a10n-claude-mock stands in for `claude`. Cobra rejects any flag it
	// does not declare (exit 1), so without these declarations the mock dies
	// before it runs. Each flag below is a REAL `claude -p` flag (confirmed
	// against the Claude Code CLI reference — see the sr:docs cite on each);
	// the mock ACCEPTS them for CLI compatibility and, except where noted, they
	// have no effect on its behaviour. A judge scenario drives the mock with a
	// --script that WRITES the verdict file (the mock executes Write/Bash tool
	// calls per its scenario) and streams an assistant/result frame; sr-agent's
	// verifier then reads that file. So these flags only need ACCEPTING.
	//
	// --model: alias (haiku/sonnet/opus/fable) or full model name. Always passed
	// by BuildInvocation. sr:docs https://code.claude.com/docs/en/cli-reference#--model
	cmd.Flags().String("model", "", "Accepted for CLI compatibility; has no effect")
	// --allowed-tools (alias --allowedTools): comma/space-separated tool names
	// allowed without prompting. The judge passes `--allowed-tools Write`.
	// Declared as a repeatable string array to mirror claude's variadic
	// `<tools...>` (also accepts a single comma-joined value). Both the kebab and
	// the camelCase spelling are real claude aliases; declare both so either is
	// accepted. sr:docs https://code.claude.com/docs/en/cli-reference#--allowed-tools
	cmd.Flags().StringArray("allowed-tools", nil, "Accepted for CLI compatibility; has no effect")
	cmd.Flags().StringArray("allowedTools", nil, "Accepted for CLI compatibility; alias of --allowed-tools; has no effect")
	// --disallowed-tools (alias --disallowedTools): the deny counterpart. Not
	// passed by the judge today, but reachable via sr-agent's --claude-args
	// pass-through, so accept it too.
	// sr:docs https://code.claude.com/docs/en/cli-reference#--disallowed-tools
	cmd.Flags().StringArray("disallowed-tools", nil, "Accepted for CLI compatibility; has no effect")
	cmd.Flags().StringArray("disallowedTools", nil, "Accepted for CLI compatibility; alias of --disallowed-tools; has no effect")
	// --permission-mode: default|acceptEdits|plan|auto|bypassPermissions|dontAsk.
	// Reachable via --claude-args (e.g. '{"permission-mode":"plan"}').
	// sr:docs https://code.claude.com/docs/en/cli-reference#--permission-mode
	cmd.Flags().String("permission-mode", "", "Accepted for CLI compatibility; has no effect")
	// --settings: a settings file path OR an inline JSON string.
	// sr:docs https://code.claude.com/docs/en/cli-reference#--settings
	cmd.Flags().String("settings", "", "Accepted for CLI compatibility; has no effect")
	// --append-system-prompt: appended to (not replacing) the default system
	// prompt. Distinct from --system-prompt (declared above), which replaces it.
	// sr:docs https://code.claude.com/docs/en/cli-reference#--append-system-prompt
	cmd.Flags().String("append-system-prompt", "", "Accepted for CLI compatibility; has no effect")
	// --input-format: text|stream-json. Mirror of the existing --output-format.
	// sr:docs https://code.claude.com/docs/en/cli-reference#--input-format
	cmd.Flags().String("input-format", "", "Accepted for CLI compatibility; has no effect")
	// --include-partial-messages: streams partial message chunks; real claude
	// requires --output-format stream-json + --print. Accepted, no effect.
	// sr:docs https://code.claude.com/docs/en/cli-reference#--include-partial-messages
	cmd.Flags().Bool("include-partial-messages", false, "Accepted for CLI compatibility; has no effect")
	// --max-budget-usd: caps API spend. sr-agent's --claude-args carries it in
	// its own tests ('{"max-budget-usd":5}'), so a judge caller may pass it.
	// sr:docs https://code.claude.com/docs/en/cli-reference#--max-budget-usd
	cmd.Flags().String("max-budget-usd", "", "Accepted for CLI compatibility; has no effect")
	// NOTE on --permission-prompt-tool: it does NOT exist in the real Claude Code
	// CLI (confirmed absent from `claude --help` and the CLI reference), so it is
	// deliberately NOT declared here — the mock accepts only flags real claude
	// accepts.
}

// backgroundTasksDisabled is whether CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1
// turns the background task functionality off. Read once here, with the rest
// of the configuration.
// sr:docs https://code.claude.com/docs/en/tools-reference#background-commands
// sr:provides background-bash/claude
func backgroundTasksDisabled() bool {
	return os.Getenv("CLAUDE_CODE_DISABLE_BACKGROUND_TASKS") == "1"
}
