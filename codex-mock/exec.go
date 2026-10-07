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

// newExec is `exec`, the non-interactive run. It knows the flags a real
// `codex exec` takes, and refuses the ones it implements nothing of
// (unimplemented.go) instead of ignoring them.
func newExec() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec [flags] [prompt]",
		Short: "Run the scenario script as a non-interactive Codex session",
		RunE:  runExec,
	}
	execFlags(cmd)
	return cmd
}

// runExec is a run of `exec`; `exec resume <session id> [prompt]` continues that
// session, in whichever directory it runs (only a session named by its id is
// modeled, not `--last`).
//
// sr:provides session-resume/codex
func runExec(cmd *cobra.Command, args []string) error {
	resume := ""
	if len(args) >= 2 && args[0] == "resume" {
		resume, args = args[1], args[2:]
	}
	f := cmd.Flags()
	if err := refuseUnimplemented(f); err != nil {
		return err
	}
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
	if abs, err := filepath.Abs(cwd); err == nil { // -C may be relative to where the mock was started
		cwd = abs
	}
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	if err := checkRepo(cmd, cwd); err != nil {
		return err
	}
	// a safety limit of the mock, not codex's behaviour: codex falls back to ~/.codex, but the mock installs
	// plugins and writes sessions and trust under its home, and must never touch the user's real one
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		return errors.New("codex-mock: CODEX_HOME is required (the mock never uses ~/.codex)")
	}
	on := func(name string) bool { return f.Lookup(name).Value.String() == "true" }
	asJSON, bypass, ephemeral := on("json"), on("dangerously-bypass-hook-trust"), on("ephemeral")
	sandbox, err := sandboxOf(f)
	if err != nil {
		return err
	}
	disabled, err := hooksDisabled(f)
	if err != nil {
		return err
	}
	model, _ := f.GetString("model")
	if resume != "" { // an unknown session fails before anything starts: no hook fires
		if err := session.ResumeUnknown(home, resume); err != nil {
			return err
		}
	}
	overrides, _ := f.GetStringArray("config")
	runner.Agents = runner.AgentSettings{MaxDepth: maxDepthOf(overrides), Script: os.Getenv("A10N_MOCK_SUBAGENT_SCRIPT")}
	// `exec fork <session id> [prompt]` continues that session in a new one
	var forkFrom string
	if len(args) >= 2 && args[0] == "fork" {
		forkFrom, args = args[1], args[2:]
	}
	return runner.Run(cmd.Context(), runner.Config{
		Script: script, Prompt: strings.Join(args, " "), Resume: resume, ForkFrom: forkFrom, Ephemeral: ephemeral, Cwd: cwd, CodexHome: home, Model: model,
		Environ: os.Environ(), JSON: asJSON, BypassHookTrust: bypass, DisableHooks: disabled, IgnoreUserConfig: on("ignore-user-config"), Sandbox: sandbox, Stdout: cmd.OutOrStdout(), Stderr: os.Stderr,
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
