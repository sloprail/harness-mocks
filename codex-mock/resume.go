package main

import (
	"errors"

	"github.com/spf13/cobra"
)

// newResume is `exec resume <session-id> [prompt]`: the session continues from
// its rollout, in whichever directory the command runs. Only a session named
// by its id is modeled; `--last` (the newest session of the directory) is not.
//
// sr:provides session-resume/codex
func newResume() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resume [flags] <session-id> [prompt]",
		Short: "Continue a recorded session by its id",
		Args:  cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if last, _ := cmd.Flags().GetBool("last"); last || len(args) == 0 {
				return errors.New("codex-mock: only a session named by its id is modeled: pass `resume <session-id>`")
			}
			return execute(cmd, args[0], args[1:])
		},
	}
	cmd.Flags().Bool("last", false, "not modeled")
	cmd.Flags().Bool("all", false, "Accepted; no effect")
	return cmd
}
