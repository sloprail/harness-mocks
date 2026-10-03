// Package runner is one cursor-mock run: the session, the scenario script's
// turns, each tool call with its hooks, and the stream it prints.
package runner

import "io"

// Config is everything a run needs, read once at the mock's entrypoint.
type Config struct {
	// Script is the scenario script that plays the agent.
	Script string
	// Prompt is the user's prompt.
	Prompt string
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
	// Version is the Cursor version the mock reports in hook payloads.
	Version string
	// Force: the run was started with --force or --yolo, which approves every
	// shell command; without it a command is rejected (recorded:
	// runs/noninteractive-no-force).
	Force  bool
	Stdout io.Writer
	Stderr io.Writer
}
