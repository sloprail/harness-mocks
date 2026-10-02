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
	// Dir is the workspace: where the run starts and where project hooks live.
	Dir string
	// Environ is the mock's own environment, which its children inherit.
	Environ []string
	// Home is the user's home, where Cursor keeps ~/.cursor.
	Home string
	// Version is the Cursor version the mock reports in hook payloads.
	Version string
	Stdout  io.Writer
	Stderr  io.Writer
}
