// Package runner is one cursor-mock run: the session, the scenario script's
// turns, each tool call with its hooks, and the stream it prints.
package runner

import (
	"io"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/hooks"
)

// loadHooks is the hooks loader of the run: the TUI also loads the user's local plugins, and the
// stop opt-in has the TUI's rules for which plugin hooks run.
func (c Config) loadHooks() func(dir, home string, pluginDirs ...string) (hooks.Config, error) {
	switch {
	case c.Interactive:
		return hooks.LoadInteractive
	case c.Stop:
		return hooks.LoadStopOptIn
	}
	return hooks.Load
}

// Config is everything a run needs, read once at the mock's entrypoint.
type Config struct {
	// Script is the scenario script that plays the agent.
	Script string
	// Prompt is the user's prompt.
	Prompt string
	// Interactive: the run is a TUI session (cursor-agent started without -p), which
	// prints no stream and fires the hooks of its turns (interactive.go).
	Interactive bool
	// Stop is the opt-in that makes a print-mode run fire what only the TUI fires at the end of a
	// turn, afterAgentResponse and stop, and continue on a stop hook's followup_message: a deviation
	// (cursor-agent -p fires neither), for tests of what a stop hook does.
	Stop bool
	// Inputs are what the user types in the TUI, line by line, in order: a prompt, or a slash
	// command typed at the idle input (only /compress, which the TUI compacts the conversation
	// for). The first is a prompt, and is Prompt.
	Inputs []string
	// Resume is the id of the session to continue, empty for a new one.
	Resume string
	// Dir is the workspace: where the run starts and where project hooks live.
	Dir string
	// Environ is the mock's own environment, which its children inherit.
	Environ []string
	// Home is the user's home, where Cursor keeps ~/.cursor.
	Home string
	// PluginDirs are the plugin directories loaded with --plugin-dir.
	PluginDirs []string
	// ApproveMCPs is --approve-mcps: the project's MCP servers are approved
	// without asking (the only mode MCP calls were recorded in).
	ApproveMCPs bool
	// Model is --model: the model the run was started with; "" is the default.
	Model string
	// Version is the Cursor version the mock reports in hook payloads.
	Version string
	// Force: the run was started with --force or --yolo, which approves every
	// shell command; without it a command is rejected (recorded:
	// runs/noninteractive-no-force).
	Force  bool
	Stdout io.Writer
	Stderr io.Writer
}
