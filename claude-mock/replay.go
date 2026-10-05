package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sloprail/harness-mocks/claude-mock/internal/replay"
)

// newReplay is `replay <run-dir>`: the mock replays a recorded run (its setup
// and the model's own turns, as a scenario script it generates) and compares
// its whole event stream and hook payloads with the recording's. It exits 0
// when they match, 1 when they differ, 2 when the recording cannot be replayed
// yet. --print-script shows the generated script and runs nothing.
func newReplay() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replay [--print-script] <run-dir>",
		Short: "Replay a recorded run and compare the mock's output with the recording",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if print, _ := cmd.Flags().GetBool("print-script"); print {
				s, err := replay.Script(args[0])
				if err != nil {
					return err
				}
				fmt.Fprint(cmd.OutOrStdout(), s)
				return nil
			}
			self, err := os.Executable()
			if err != nil {
				return err
			}
			diff, err := replay.Run(self, args[0], os.Environ())
			if u, ok := err.(*replay.Unbuildable); ok {
				fmt.Fprintf(cmd.OutOrStdout(), "not replayable: %s\n", u.Reason)
				os.Exit(2)
			}
			if err != nil {
				return err
			}
			if diff != "" {
				fmt.Fprint(cmd.OutOrStdout(), diff)
				os.Exit(1)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "replays green")
			return nil
		},
	}
	cmd.Flags().Bool("print-script", false, "Print the generated scenario script and run nothing")
	return cmd
}
