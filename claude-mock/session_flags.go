package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sloprail/harness-mocks/claude-mock/internal/runner"
)

const (
	flagContinue = "continue"
	flagName     = "name"
)

// resolveSessionFlags names the session a run resumes before the run reads its
// flags: --resume takes an id, the path of a session's transcript file or a
// session's name, and --continue, with no --resume, resumes the directory's most
// recent session. Both become the id --resume carries. --name names the session
// the run writes.
//
// sr:provides session-resume/claude
func resolveSessionFlags(cmd *cobra.Command, _ []string) error {
	if name, _ := cmd.Flags().GetString(flagName); name != "" {
		_ = os.Setenv(runner.EnvSessionName, name)
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
	if cont, _ := cmd.Flags().GetBool(flagContinue); cont && resume == "" {
		if resume = runner.LatestSession(configDir, projectDir); resume == "" {
			return fmt.Errorf("claude-mock: --continue: no conversation found to continue in %s", projectDir)
		}
	}
	if resume == "" {
		return nil
	}
	return cmd.Flags().Set(flagResume, runner.ResumeTarget(configDir, projectDir, resume))
}
