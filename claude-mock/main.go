package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sloprail/harness-mocks/claude-mock/internal/runner"
	"github.com/sloprail/harness-mocks/internal/procexec"
)

// version is the build version, stamped by the release workflow with
// -ldflags "-X main.version=<tag without v>"; "dev" for any other build.
var version = "dev"

const (
	flagResumeLookup  = "resume-lookup" // hidden: the --resume value was a name or path, or --continue
	flagContinue      = "continue"
	flagNoPersistence = "no-session-persistence"
)

// forgetUnpersisted is what a run with --no-session-persistence ends with: no
// session left to resume.
//
// sr:provides session-resume/claude
func forgetUnpersisted(cmd *cobra.Command, _ []string) error {
	if off, _ := cmd.Flags().GetBool(flagNoPersistence); !off {
		return nil
	}
	id, _ := cmd.Flags().GetString(flagSessionID)
	projectDir, _ := cmd.Flags().GetString(flagProjectDir)
	if projectDir == "" {
		projectDir, _ = os.Getwd()
	}
	configDir, _ := cmd.Flags().GetString(flagConfigDir)
	runner.ForgetSession(configDir, projectDir, id)
	return nil
}

// resolveSessionFlags names the session a run resumes before the run reads its
// flags: --resume takes an id, the path of a session's transcript file or a
// session's name, and --continue, with no --resume, resumes the directory's most
// recent session, or starts a new one when it has none. The resumed ones become the
// id --resume carries.
//
// sr:provides session-resume/claude
func resolveSessionFlags(cmd *cobra.Command, args []string) error {
	if err := refuseUnimplemented(cmd, args); err != nil {
		return err
	}
	resume, _ := cmd.Flags().GetString(flagResume)
	projectDir, _ := cmd.Flags().GetString(flagProjectDir)
	if projectDir == "" {
		var err error
		if projectDir, err = os.Getwd(); err != nil {
			return fmt.Errorf("claude-mock: getwd: %w", err)
		}
	}
	configDir, _ := cmd.Flags().GetString(flagConfigDir)
	lookup := false
	if cont, _ := cmd.Flags().GetBool(flagContinue); cont && resume == "" {
		lookup = true
		if resume = runner.LatestSession(configDir, projectDir); resume == "" {
			// no session to continue: a new one starts (recorded: snapshots/runs/resume-continue-none)
			if id, _ := cmd.Flags().GetString(flagSessionID); id == "" {
				return cmd.Flags().Set(flagSessionID, runner.NewSessionID())
			}
		}
	}
	if resume == "" {
		return nil
	}
	target := runner.ResumeTarget(configDir, projectDir, resume)
	// a session named by anything but its id is looked up, and the stream tells the lookup's own id (recorded: runs/resume-name, resume-path, resume-continue)
	if err := cmd.Flags().Set(flagResumeLookup, fmt.Sprint(lookup || target != resume)); err != nil {
		return err
	}
	return cmd.Flags().Set(flagResume, target)
}

func main() {
	procexec.KillDetachedOnSignal()
	err := newRoot().Execute()
	procexec.KillDetached() // a Bash job outlives its call, not the mock (adr/child-processes)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "a10n-claude-mock",
		Short: "Deterministic Claude Code mock for e2e testing",
		Long: `a10n-claude-mock is a drop-in replacement for the 'claude' CLI binary used in
end-to-end tests. Instead of calling an LLM, it executes a user-supplied shell
script that streams Claude Code-compatible JSONL to stdout. It validates the
output, fires Claude Code lifecycle hooks (from .claude/settings.json), and
passes the JSONL through — so the calling system sees exactly what it would
from a real Claude Code session.

Usage as a claude replacement:

  claude -p --output-format stream-json --session-id <id> <prompt>
  →
  a10n-claude-mock -p --output-format stream-json --session-id <id> --script scenario.sh <prompt>

Or point A10N_MOCK_SCRIPT at the script instead of passing --script each time.`,
		// --version reports this build of the mock, nothing else. The mock never
		// answered --version before (it is not part of any scenario), so there is
		// no real-claude --version behaviour to preserve.
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		// claude -p is the primary entrypoint; support it at the root.
		// the prompt is the positional arguments; `replay` is the one subcommand
		Args:     cobra.ArbitraryArgs,
		PreRunE:  resolveSessionFlags,
		RunE:     rootRunE,
		PostRunE: forgetUnpersisted,
	}

	root.SetVersionTemplate("{{.Version}}\n")
	addRunFlags(root)
	addRefusedFlags(root)
	root.SetFlagErrorFunc(flagError)
	root.AddCommand(newReplay())

	return root
}
