package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Entry is one hook configured for an event.
type Entry struct {
	// Command is the shell line to run.
	Command string
	// FailClosed: a failure of the hook (a non-zero exit status, no output)
	// blocks the action instead of letting it through.
	FailClosed bool
	// Matcher is a regular expression the event's subject must match for the
	// hook to run: "" and "*" match everything.
	Matcher string
	// Timeout stops the command after this long; zero is none of its own, and
	// the core applies the harness's default for it (corehooks.DefaultTimeout).
	Timeout time.Duration
}

// Config is the project's hooks: the entries configured for each event, in
// order.
type Config struct {
	entries map[Event][]Entry
}

// Load reads <dir>/.cursor/hooks.json, then the hooks of each plugin directory
// loaded with --plugin-dir (plugin.go). No file is no hooks. A hook's command,
// failClosed, matcher and timeout are modeled; its loop_limit, the prompt type
// of hook and the user, team and enterprise sources are not.
//
// sr:docs https://cursor.com/docs/hooks#configuration
func Load(dir string, pluginDirs ...string) (Config, error) {
	c := Config{entries: map[Event][]Entry{}}
	if err := c.addFile(filepath.Join(dir, ".cursor", "hooks.json")); err != nil {
		return Config{}, err
	}
	for _, p := range pluginDirs {
		if err := c.addPlugin(dir, p); err != nil {
			return Config{}, err
		}
	}
	return c, nil
}

// addFile adds the hooks of one hooks.json after those already configured; a
// missing file adds none.
func (c Config) addFile(path string) error {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var file struct {
		Hooks map[string][]struct {
			Command    string  `json:"command"`
			FailClosed bool    `json:"failClosed"`
			Matcher    string  `json:"matcher"`
			Timeout    float64 `json:"timeout"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for name, defs := range file.Hooks {
		for _, d := range defs {
			if d.Command != "" {
				c.entries[Event(name)] = append(c.entries[Event(name)], Entry{d.Command, d.FailClosed, d.Matcher, time.Duration(d.Timeout * float64(time.Second))})
			}
		}
	}
	return nil
}

// Entries are the hooks configured for the event, in order.
func (c Config) Entries(e Event) []Entry { return c.entries[e] }
