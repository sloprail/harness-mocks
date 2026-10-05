package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// lookedUp is whether the run's --resume value was found by lookup.
func lookedUp(cmd *cobra.Command) bool {
	b, _ := cmd.Flags().GetBool(flagResumeLookup)
	return b
}

// permissionMode is the mode the run's hooks are told it is in:
// bypassPermissions for --dangerously-skip-permissions, else the
// --permission-mode given, else default.
// sr:docs https://code.claude.com/docs/en/hooks#common-input-fields
func permissionMode(cmd *cobra.Command) string {
	if skip, _ := cmd.Flags().GetBool("dangerously-skip-permissions"); skip {
		return "bypassPermissions"
	}
	if m, _ := cmd.Flags().GetString("permission-mode"); m != "" {
		return m
	}
	return "default"
}

// refuseUnimplemented is the error for an input the mock does not implement
// (adr/fail-fast-unimplemented): an output format but stream-json, which a run
// would otherwise print stream frames for as if it were one;
// --include-partial-messages; and a piped stdin, which the real run reads.
func refuseUnimplemented(cmd *cobra.Command) error {
	if f, _ := cmd.Flags().GetString(flagOutputFormat); f != "stream-json" {
		return fmt.Errorf("claude-mock: --output-format %s is not implemented by the mock (only stream-json): it is refused rather than ignored", f)
	}
	if cmd.Flags().Changed("include-partial-messages") {
		return fmt.Errorf("claude-mock: --include-partial-messages is not implemented by the mock: it is refused rather than ignored")
	}
	if fi, err := os.Stdin.Stat(); err == nil && (fi.Mode()&os.ModeNamedPipe != 0 || fi.Mode().IsRegular() && fi.Size() > 0) {
		return fmt.Errorf("claude-mock: a piped stdin is not implemented by the mock: it is refused rather than ignored")
	}
	return nil
}
