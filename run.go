package main

import (
	"fmt"
	"os"
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
	flagPrint          = "p"
)

// addRunFlags registers all flags needed to mimic the claude CLI interface.
func addRunFlags(cmd *cobra.Command) {
	cmd.Flags().String(flagScript, "", "Shell script to run as the mock agent (env: A10N_MOCK_SCRIPT)")
	cmd.Flags().String(flagSessionID, "", "Session ID (--session-id, as used by claude CLI)")
	cmd.Flags().String(flagResume, "", "Session ID to resume (--resume, as used by claude CLI)")
	cmd.Flags().String(flagOutputFormat, "stream-json", "Output format (must be stream-json)")
	cmd.Flags().String(flagProjectDir, "", "Project root for settings.json resolution (default: cwd)")
	cmd.Flags().String(flagConfigDir, "", "Claude config dir for session JSONL storage (env: CLAUDE_CONFIG_DIR, default: /tmp/a10n/claude-mock)")
	cmd.Flags().String(flagPluginCacheDir, "", "Plugin/marketplace cache root (env: CLAUDE_CODE_PLUGIN_CACHE_DIR, default: /tmp/a10n-mock-plugins)")
	cmd.Flags().BoolP(flagPrint, "p", false, "Print mode flag (passed by claude runner; accepted and ignored)")
	// claude also passes --verbose; accept but ignore.
	cmd.Flags().Bool("verbose", false, "Accepted for CLI compatibility; has no effect")
}

// rootRunE implements the root command's RunE — the primary entrypoint when the
// binary is used as a drop-in for 'claude -p --output-format stream-json ...'.
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
	prompt := strings.Join(args, " ")
	cwd, _ := os.Getwd()

	return runner.Run(cmd.Context(), runner.Config{
		ScriptPath:     scriptPath,
		SessionID:      sessionID,
		IsResume:       isResume,
		Prompt:         prompt,
		Cwd:            cwd,
		ProjectDir:     projectDir,
		ConfigDir:      configDir,
		PluginCacheDir: pluginCacheDir,
		Stderr:         os.Stderr,
		Out:            os.Stdout,
	})
}
