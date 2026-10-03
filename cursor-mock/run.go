package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/runner"
)

// version is the Cursor version the mock reports in hook payloads: the one its
// recordings were captured at.
const version = "2026.09.28-64d2043"

// run reads the command line and the environment once, into the run's
// configuration, and plays the run.
func run(cmd *cobra.Command, f flags, args []string) error {
	switch {
	case !f.print:
		return errors.New("cursor-mock: only non-interactive runs are modeled: pass -p")
	case f.outputFormat != "stream-json":
		return fmt.Errorf("cursor-mock: output format %q is not modeled: pass --output-format stream-json", f.outputFormat)
	case f.resume || f.cont:
		return errors.New("cursor-mock: --resume and --continue are not modeled")
	}
	script := f.script
	if script == "" {
		script = os.Getenv("A10N_MOCK_SCRIPT")
	}
	if script == "" {
		return errors.New("cursor-mock: no scenario script: pass --script or set A10N_MOCK_SCRIPT")
	}
	dir := f.workspace
	if dir == "" {
		var err error
		if dir, err = os.Getwd(); err != nil {
			return fmt.Errorf("cursor-mock: %w", err)
		}
	}
	// Symlinks resolved once, here: Cursor reports the canonical workspace path.
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cursor-mock: %w", err)
	}
	return runner.Run(cmd.Context(), runner.Config{
		Script: script, Prompt: strings.Join(args, " "), Dir: dir, Environ: os.Environ(), Home: home,
		Version: version, Force: f.force || f.yolo, Stdout: os.Stdout, Stderr: os.Stderr, PluginDirs: f.pluginDirs,
	})
}
