package main

import "github.com/spf13/cobra"

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
