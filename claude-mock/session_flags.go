package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sloprail/harness-mocks/claude-mock/internal/runner"
)

const flagContinue = "continue"

// sessionFor reads which session a run is for from --session-id, --resume,
// --continue and --fork-session: --resume (an id, or the path of a session's
// transcript file) takes precedence and resumes; --continue resumes the
// directory's most recent session; with --fork-session the resumed session is
// what is continued FROM and the run goes under --session-id (or a fresh id).
//
// sr:provides session-resume/claude
// sr:provides session-fork/claude
func sessionFor(cmd *cobra.Command, projectDir string) (sessionID string, isResume bool, forkFrom string, err error) {
	sessionID, _ = cmd.Flags().GetString(flagSessionID)
	resumeID, _ := cmd.Flags().GetString(flagResume)
	resumeID = runner.ResumeTarget(resumeID)
	forkSession, _ := cmd.Flags().GetBool(flagForkSession)
	if cont, _ := cmd.Flags().GetBool(flagContinue); cont && resumeID == "" {
		configDir, _ := cmd.Flags().GetString(flagConfigDir)
		if resumeID = runner.LatestSession(configDir, projectDir); resumeID == "" {
			return "", false, "", fmt.Errorf("claude-mock: --continue: no conversation found to continue in %s", projectDir)
		}
	}
	switch {
	case resumeID != "" && forkSession:
		forkFrom, isResume = resumeID, true
		if sessionID == "" {
			sessionID = runner.NewSessionID()
		}
	case resumeID != "":
		sessionID, isResume = resumeID, true
	}
	if sessionID == "" {
		return "", false, "", fmt.Errorf("claude-mock: --session-id, --resume or --continue is required")
	}
	return sessionID, isResume, forkFrom, nil
}
