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
		Use:   "a10n-codex-mock",
		Short: "Deterministic Codex CLI mock for e2e testing",
		Long: `a10n-codex-mock is a drop-in replacement for 'codex exec' in end-to-end
tests. Instead of calling a model it runs a user-supplied scenario script that
prints stream-json lines, fires Codex's hooks (from $CODEX_HOME/hooks.json and
<cwd>/.codex/hooks.json) and prints the events 'codex exec --json' prints.

  codex exec --json "prompt"
  →
  a10n-codex-mock exec --json --script scenario.sh "prompt"

Or point A10N_MOCK_SCRIPT at the script instead of passing --script each time.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newExec(), newReplay())
	return root
}
