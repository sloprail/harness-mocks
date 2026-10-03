package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sloprail/harness-mocks/claude-mock/internal/runner"
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
	flagForkSession    = "fork-session"
)

// rootRunE implements the root command's RunE — the primary entrypoint when the
// binary is used as a drop-in for 'claude -p --output-format stream-json ...'.
// sr:provides session-resume/claude
// sr:provides session-fork/claude
// sr:provides noninteractive-run/claude
// a10n:blueprint:ignore
func rootRunE(cmd *cobra.Command, args []string) error {
	scriptPath, _ := cmd.Flags().GetString(flagScript)
	if scriptPath == "" {
		scriptPath = os.Getenv("A10N_MOCK_SCRIPT")
	}

	sessionID, _ := cmd.Flags().GetString(flagSessionID)
	resumeID, _ := cmd.Flags().GetString(flagResume)
	forkSession, _ := cmd.Flags().GetBool(flagForkSession)
	isResume := false
	forkFrom := ""

	// Normalise: --resume takes precedence and sets isResume — except with
	// --fork-session, where the resumed id is what is continued FROM and the
	// session runs under --session-id (or a fresh id).
	switch {
	case resumeID != "" && forkSession:
		forkFrom = resumeID
		isResume = true
		if sessionID == "" {
			sessionID = runner.NewSessionID()
		}
	case resumeID != "":
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

	// A10N_MOCK_NO_RESUME=1 makes every --resume behave as one naming a session
	// that does not exist (see noConversation), whatever is on disk.
	// sr:invariant no-resume
	if isResume && os.Getenv("A10N_MOCK_NO_RESUME") == "1" {
		noConversation(cmd, &runner.ErrNoConversation{SessionID: sessionID})
	}

	configDir, _ := cmd.Flags().GetString(flagConfigDir)
	pluginCacheDir, _ := cmd.Flags().GetString(flagPluginCacheDir)
	printMode, _ := cmd.Flags().GetBool(flagPrint)
	systemPrompt, _ := cmd.Flags().GetString("system-prompt")
	// sr:invariant system-prompt-env
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
	// explicit directory string does so unresolved), so resolving at the getwd() fallback alone never sufficed.
	cwd := projectDir
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	model, _ := cmd.Flags().GetString("model")
	err := runner.Run(cmd.Context(), runner.Config{
		ScriptPath:              scriptPath,
		SessionID:               sessionID,
		IsResume:                isResume,
		ForkFrom:                forkFrom,
		Prompt:                  prompt,
		Cwd:                     cwd,
		ProjectDir:              projectDir,
		ConfigDir:               configDir,
		PluginCacheDir:          pluginCacheDir,
		PrintMode:               printMode,
		Model:                   model,
		BgWaitCeiling:           printWaitCeiling(),
		SpawnLimit:              spawnLimit(),
		BackgroundTasksDisabled: backgroundTasksDisabled(),
		MaxConcurrentSubagents:  os.Getenv("CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS"),
		Stderr:                  os.Stderr,
		Out:                     os.Stdout,
	})
	var noConv *runner.ErrNoConversation
	if errors.As(err, &noConv) {
		noConversation(cmd, noConv)
	}
	return err
}

// noConversation ends the run the way real Claude Code ends `--resume <id>`
// for a session it has no transcript of (claude 2.1.282): the error's message
// ("No conversation found with session ID: <id>") on stderr, an error result frame on stdout when
// the output format is stream-json, exit status 1.
//
// sr:provides session-resume-unknown/claude
func noConversation(cmd *cobra.Command, noConv *runner.ErrNoConversation) {
	sessionID, msg := noConv.SessionID, noConv.Error()
	fmt.Fprintln(os.Stderr, msg)
	if format, _ := cmd.Flags().GetString(flagOutputFormat); format == "stream-json" {
		frame, _ := json.Marshal(map[string]any{
			"type": "result", "subtype": "error_during_execution", "is_error": true,
			"num_turns": 0, "session_id": sessionID, "errors": []string{msg},
		})
		fmt.Println(string(frame))
	}
	os.Exit(1)
}
