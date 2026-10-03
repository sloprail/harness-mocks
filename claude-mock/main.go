package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := newRoot().Execute(); err != nil {
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
		SilenceUsage:  true,
		SilenceErrors: true,
		// claude -p is the primary entrypoint; support it at the root.
		PreRunE: resolveSessionFlags,
		RunE:    rootRunE,
	}

	addRunFlags(root)

	return root
}
