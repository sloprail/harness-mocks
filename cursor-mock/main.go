// Command a10n-cursor-mock is a deterministic stand-in for the cursor-agent
// CLI: instead of calling a model it runs a scenario script that prints
// Cursor-compatible stream-json, fires Cursor's hooks from .cursor/hooks.json,
// and runs the tool calls the script asks for.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := newRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
