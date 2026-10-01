package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sloprail/harness-mocks/codex-mock/internal/runner"
)

// newExec is `exec`, the non-interactive run. It accepts the flags a real
// `codex exec` accepts and has no use for, so a caller's command line works
// unchanged.
func newExec() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec [flags] [prompt]",
		Short: "Run the scenario script as a non-interactive Codex session",
		RunE:  runExec,
	}
	f := cmd.Flags()
	f.String("script", "", "Scenario script that drives the agent (env: A10N_MOCK_SCRIPT)")
	f.Bool("json", false, "Print events to stdout as JSONL")
	f.StringP("cd", "C", "", "Working directory of the session (default: the current one)")
	f.StringP("model", "m", "", "Model name reported in hook payloads")
	// Accepted for compatibility with `codex exec`; no effect.
	f.StringArrayP("config", "c", nil, "Accepted; no effect")
	f.StringArray("enable", nil, "Accepted; no effect")
	f.StringArray("disable", nil, "Accepted; no effect")
	f.StringP("sandbox", "s", "", "Accepted; no effect")
	f.StringP("profile", "p", "", "Accepted; no effect")
	f.String("color", "", "Accepted; no effect")
	f.StringP("output-last-message", "o", "", "Accepted; no effect")
	f.String("output-schema", "", "Accepted; no effect")
	f.String("thread-source", "", "Accepted; no effect")
	for _, name := range []string{"skip-git-repo-check", "ephemeral", "ignore-user-config", "ignore-rules",
		"strict-config", "dangerously-bypass-approvals-and-sandbox", "dangerously-bypass-hook-trust", "approve-for-me"} {
		f.Bool(name, false, "Accepted; no effect")
	}
	return cmd
}

func runExec(cmd *cobra.Command, args []string) error {
	f := cmd.Flags()
	script, _ := f.GetString("script")
	if script == "" {
		script = os.Getenv("A10N_MOCK_SCRIPT")
	}
	cwd, _ := f.GetString("cd")
	if cwd == "" {
		var err error
		if cwd, err = os.Getwd(); err != nil {
			return fmt.Errorf("codex-mock: getwd: %w", err)
		}
	}
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("codex-mock: no CODEX_HOME and no home directory: %w", err)
		}
		home = filepath.Join(user, ".codex")
	}
	asJSON, _ := f.GetBool("json")
	model, _ := f.GetString("model")
	return runner.Run(cmd.Context(), runner.Config{
		Script: script, Prompt: strings.Join(args, " "), Cwd: cwd, CodexHome: home, Model: model,
		Environ: os.Environ(), JSON: asJSON, Stdout: cmd.OutOrStdout(), Stderr: os.Stderr,
	})
}
