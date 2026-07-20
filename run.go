package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/a10n-build/a10n-cli/services/claude-mock/internal/runner"
)

// Flag names shared between root (when used as claude replacement) and any future subcommands.
const (
	flagScript         = "script"
	flagSessionID      = "session-id"
	flagResume         = "resume"
	flagOutputFormat   = "output-format"
	flagProjectDir     = "project-dir"
	flagConfigDir      = "config-dir"
	flagPluginCacheDir = "plugin-cache-dir"
	flagPrint          = "print"
)

// addRunFlags registers all flags needed to mimic the claude CLI interface.
// a10n:blueprint:ignore
func addRunFlags(cmd *cobra.Command) {
	cmd.Flags().String(flagScript, "", "Shell script to run as the mock agent (env: A10N_MOCK_SCRIPT)")
	cmd.Flags().String(flagSessionID, "", "Session ID (--session-id, as used by claude CLI)")
	cmd.Flags().String(flagResume, "", "Session ID to resume (--resume, as used by claude CLI)")
	cmd.Flags().String(flagOutputFormat, "stream-json", "Output format (must be stream-json)")
	cmd.Flags().String(flagProjectDir, "", "Project root for settings.json resolution (default: cwd)")
	cmd.Flags().String(flagConfigDir, "", "Claude config dir for session JSONL storage (env: CLAUDE_CONFIG_DIR, default: /tmp/a10n/claude-mock)")
	cmd.Flags().String(flagPluginCacheDir, "", "Plugin/marketplace cache root (env: CLAUDE_CODE_PLUGIN_CACHE_DIR, default: /tmp/a10n-mock-plugins)")
	// --print activates non-interactive print mode: the script's raw stdout is
	// forwarded directly (no JSONL parsing, no session persistence).
	// This mirrors `claude --print` used by the autopilot supervisor.
	// a10n:docs https://code.claude.com/docs/en/cli-reference#--print
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
}

// rootRunE implements the root command's RunE — the primary entrypoint when the
// binary is used as a drop-in for 'claude -p --output-format stream-json ...'.
// a10n:blueprint:ignore
func rootRunE(cmd *cobra.Command, args []string) error {
	scriptPath, _ := cmd.Flags().GetString(flagScript)
	if scriptPath == "" {
		scriptPath = os.Getenv("A10N_MOCK_SCRIPT")
	}

	sessionID, _ := cmd.Flags().GetString(flagSessionID)
	resumeID, _ := cmd.Flags().GetString(flagResume)
	isResume := false

	// Normalise: --resume takes precedence and sets isResume.
	if resumeID != "" {
		sessionID = resumeID
		isResume = true
	}
	if sessionID == "" {
		return fmt.Errorf("claude-mock: --session-id or --resume is required")
	}

	projectDir, _ := cmd.Flags().GetString(flagProjectDir)
	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("claude-mock: getwd: %w", err)
		}
	}

	// If --resume is used for a session that "doesn't exist", the real claude emits
	// an error result and exits 1. The mock supports this via A10N_MOCK_NO_RESUME:
	// if set, --resume behaves as a probe failure (exits 1 with an error result).
	if isResume && os.Getenv("A10N_MOCK_NO_RESUME") == "1" {
		fmt.Printf(`{"type":"result","subtype":"error_during_execution","is_error":true,"errors":["No conversation found with session ID: %s"]}`, sessionID)
		fmt.Println()
		os.Exit(1)
	}

	configDir, _ := cmd.Flags().GetString(flagConfigDir)
	pluginCacheDir, _ := cmd.Flags().GetString(flagPluginCacheDir)
	printMode, _ := cmd.Flags().GetBool(flagPrint)
	systemPrompt, _ := cmd.Flags().GetString("system-prompt")
	if systemPrompt != "" {
		os.Setenv("A10N_MOCK_SYSTEM_PROMPT", systemPrompt) //nolint:errcheck
	}
	prompt := strings.Join(args, " ")
	// Use projectDir as cwd when explicitly provided — it is the directory the
	// simulated claude session runs in (the same as what real claude uses).
	// Fall back to os.Getwd() only when --project-dir is not set.
	//
	// Symlink-resolved ONCE, here, at the single source every Config.Cwd downstream use
	// derives from (transcript-path encoding, hook payload `cwd` fields, the nested
	// subagent run's own Cwd) — matching real Claude Code, which resolves symlinks
	// consistently everywhere (verified empirically: a real claude run's own PreToolUse
	// payload `cwd` field is ALREADY the resolved form, e.g. /private/tmp/... on macOS, not
	// /tmp/...). --project-dir is passed as an unresolved path far more often than a plain
	// os.Getwd() fallback would ever be (every test harness/caller that hands the mock an
	// explicit directory string does so unresolved), so resolving only at the getwd()
	// fallback branch above was never enough on its own.
	cwd := projectDir
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}

	return runner.Run(cmd.Context(), runner.Config{
		ScriptPath:     scriptPath,
		SessionID:      sessionID,
		IsResume:       isResume,
		Prompt:         prompt,
		Cwd:            cwd,
		ProjectDir:     projectDir,
		ConfigDir:      configDir,
		PluginCacheDir: pluginCacheDir,
		PrintMode:      printMode,
		Stderr:         os.Stderr,
		Out:            os.Stdout,
	})
}
