package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sloprail/harness-mocks/codex-mock/internal/runner"
	"github.com/sloprail/harness-mocks/codex-mock/internal/session"
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
	f := cmd.PersistentFlags()
	f.String("script", "", "Scenario script that drives the agent (env: A10N_MOCK_SCRIPT)")
	f.Bool("json", false, "Print events to stdout as JSONL")
	f.StringP("cd", "C", "", "Working directory of the session (default: the current one)")
	f.StringP("model", "m", "", "Model name reported in hook payloads")
	// Accepted for compatibility with `codex exec`; no effect.
	f.StringArrayP("config", "c", nil, "Config override key=value; only agents.max_depth has an effect")
	f.StringArray("enable", nil, "Accepted; no effect")
	f.StringArray("disable", nil, "Accepted; no effect")
	f.StringP("sandbox", "s", "", "Accepted; no effect")
	f.StringP("profile", "p", "", "Accepted; no effect")
	f.String("color", "", "Accepted; no effect")
	f.StringP("output-last-message", "o", "", "Accepted; no effect")
	f.String("output-schema", "", "Accepted; the final message is the script's, not shaped by the schema")
	f.String("thread-source", "", "Accepted; no effect")
	for _, name := range []string{"skip-git-repo-check", "ephemeral", "ignore-user-config", "ignore-rules",
		"strict-config", "dangerously-bypass-approvals-and-sandbox", "dangerously-bypass-hook-trust", "approve-for-me"} {
		f.Bool(name, false, "Accepted; no effect")
	}
	cmd.AddCommand(newResume())
	return cmd
}

func runExec(cmd *cobra.Command, args []string) error { return execute(cmd, "", args) }

// execute is a run of `exec`, resuming session resume when it is not empty.
func execute(cmd *cobra.Command, resume string, args []string) error {
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
	if err := checkRepo(cmd, cwd); err != nil {
		return err
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
	bypass, _ := f.GetBool("dangerously-bypass-hook-trust")
	model, _ := f.GetString("model")
	if resume != "" { // an unknown session fails before anything starts: no hook fires
		if err := session.ResumeUnknown(home, resume); err != nil {
			return err
		}
	}
	overrides, _ := f.GetStringArray("config")
	runner.Agents = runner.AgentSettings{MaxDepth: maxDepthOf(overrides), Script: os.Getenv("A10N_MOCK_SUBAGENT_SCRIPT")}
	return runner.Run(cmd.Context(), runner.Config{
		Script: script, Prompt: strings.Join(args, " "), Resume: resume, Cwd: cwd, CodexHome: home, Model: model,
		Environ: os.Environ(), JSON: asJSON, BypassHookTrust: bypass, Stdout: cmd.OutOrStdout(), Stderr: os.Stderr,
	})
}

// errNotTrusted is what Codex prints, and exits 1 on, when a run starts
// outside a repository without being told it may
// (runs/noninteractive-run-git-check-refused).
var errNotTrusted = errors.New("Not inside a trusted directory and --skip-git-repo-check was not specified.")

// checkRepo refuses a run whose directory is not inside a repository, unless
// --skip-git-repo-check is given or the run bypasses approvals and the
// sandbox: the recording of that flag (runs/noninteractive-run-no-git-check)
// shows Codex running outside a repository without complaint. Trusting a
// directory through the user's config is not modeled.
// sr:provides noninteractive-run/codex
func checkRepo(cmd *cobra.Command, cwd string) error {
	f := cmd.Flags()
	if skip, _ := f.GetBool("skip-git-repo-check"); skip {
		return nil
	}
	if bypass, _ := f.GetBool("dangerously-bypass-approvals-and-sandbox"); bypass {
		return nil
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil { // a directory, or a file in a worktree
			return nil
		}
		if filepath.Dir(dir) == dir {
			return errNotTrusted
		}
	}
}

// maxDepthOf is the agents.max_depth a -c override sets (the last one wins),
// 0 when none does: Codex's default depth.
func maxDepthOf(overrides []string) int {
	depth := 0
	for _, o := range overrides {
		if v, ok := strings.CutPrefix(o, "agents.max_depth="); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				depth = n
			}
		}
	}
	return depth
}
