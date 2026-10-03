package main

import (
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
	f := cmd.Flags()
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
	bypass, _ := f.GetBool("dangerously-bypass-hook-trust")
	model, _ := f.GetString("model")
	if len(args) >= 2 && args[0] == "resume" { // `exec resume <session id> [prompt]`
		return session.Resume(home, args[1])
	}
	overrides, _ := f.GetStringArray("config")
	runner.Agents = runner.AgentSettings{MaxDepth: maxDepthOf(overrides), Script: os.Getenv("A10N_MOCK_SUBAGENT_SCRIPT")}
	return runner.Run(cmd.Context(), runner.Config{
		Script: script, Prompt: strings.Join(args, " "), Cwd: cwd, CodexHome: home, Model: model,
		Environ: os.Environ(), JSON: asJSON, BypassHookTrust: bypass, Stdout: cmd.OutOrStdout(), Stderr: os.Stderr,
	})
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
