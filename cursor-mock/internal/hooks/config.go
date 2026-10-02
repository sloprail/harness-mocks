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

// Load reads <dir>/.cursor/hooks.json. No file is no hooks. A hook's command,
// failClosed, matcher and timeout are modeled; its loop_limit, the prompt type
// of hook and the user, team and enterprise sources are not.
//
// sr:docs https://cursor.com/docs/hooks#configuration
func Load(dir string) (Config, error) {
	path := filepath.Join(dir, ".cursor", "hooks.json")
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
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
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	c := Config{entries: map[Event][]Entry{}}
	for name, defs := range file.Hooks {
		for _, d := range defs {
			if d.Command != "" {
				c.entries[Event(name)] = append(c.entries[Event(name)], Entry{d.Command, d.FailClosed, d.Matcher, time.Duration(d.Timeout * float64(time.Second))})
			}
		}
	}
	return c, nil
}

// Entries are the hooks configured for the event, in order.
func (c Config) Entries(e Event) []Entry { return c.entries[e] }
