package main

import "github.com/spf13/cobra"

// execFlags registers the flags of `codex exec`: those the mock implements, and those it takes only to refuse them.
func execFlags(cmd *cobra.Command) {
	f := cmd.PersistentFlags()
	f.String("script", "", "Scenario script that drives the agent (env: A10N_MOCK_SCRIPT)")
	f.Bool("json", false, "Print events to stdout as JSONL")
	f.StringP("cd", "C", "", "Working directory of the session (default: the current one)")
	f.StringP("model", "m", "", "Model name reported in hook payloads")
	f.StringArrayP("config", "c", nil, "Config override key=value; only agents.max_depth is implemented")
	f.StringArray("enable", nil, "Not implemented: refused")
	f.StringArray("disable", nil, "Not implemented: refused")
	f.StringP("sandbox", "s", "", "Not implemented: refused")
	f.StringP("profile", "p", "", "Not implemented: refused")
	f.String("color", "", "Not implemented: refused")
	f.StringP("output-last-message", "o", "", "Not implemented: refused")
	f.String("output-schema", "", "Not implemented: refused")
	f.String("thread-source", "", "Not implemented: refused")
	for _, name := range []string{"ignore-user-config", "ignore-rules", "strict-config", "approve-for-me"} {
		f.Bool(name, false, "Not implemented: refused")
	}
	for _, name := range []string{"skip-git-repo-check", "dangerously-bypass-approvals-and-sandbox", "dangerously-bypass-hook-trust", "ephemeral"} {
		f.Bool(name, false, "Implemented")
	}
}
