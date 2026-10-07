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
// configuration, and plays the run. --resume <id> continues that session: the
// run is told its id (recorded: runs/session-resume).
//
// sr:provides session-resume/cursor
func run(cmd *cobra.Command, f flags, args []string) error {
	prompt, typed := strings.Join(args, " "), []string(nil)
	if !f.print { // cursor-agent without -p is the TUI: the prompt is typed, so it is read from stdin
		var err error
		if prompt, typed, err = typedPrompt(args, os.Stdin); err != nil {
			return err
		}
	}
	switch {
	case !f.print && (f.outputFormat != "text" || f.resume != "" || len(f.addDirs) > 0 || len(f.pluginDirs) > 0 || f.approveMCPs):
		return errors.New("cursor-mock: a TUI session is modeled with a prompt on stdin and no -p, --output-format, --resume, --add-dir, --plugin-dir or --approve-mcps: only turns of text were recorded")
	case f.print && f.outputFormat != "stream-json":
		return fmt.Errorf("cursor-mock: output format %q is not modeled: pass --output-format stream-json", f.outputFormat)
	case f.cont:
		return errors.New("cursor-mock: --continue is not modeled: pass --resume <session-id>")
	case f.model != "" && f.model != "auto" && f.model != "cursor-grok-4.5-high":
		return fmt.Errorf("cursor-mock: model %q is not modeled: only auto and cursor-grok-4.5-high were recorded", f.model)
	case len(f.addDirs) > 0 && !f.force:
		return errors.New("cursor-mock: --add-dir is modeled only with --force (the mode its recordings cover): pass --force")
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
		Script: script, Prompt: prompt, Interactive: !f.print, Typed: typed, Resume: f.resume, Dir: dir, Environ: os.Environ(), Home: home,
		Version: version, Force: f.force || f.yolo, Stdout: os.Stdout, Stderr: os.Stderr, PluginDirs: f.pluginDirs, ApproveMCPs: f.approveMCPs, Model: f.model,
	})
}
