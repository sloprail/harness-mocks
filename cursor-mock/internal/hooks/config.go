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
	// PluginRoot is the directory of the plugin the hook comes from, "" for a project's or the
	// user's: a plugin's hook runs there, with CURSOR_PLUGIN_ROOT set to it (recorded:
	// runs/plugin-hook-cwd-env).
	PluginRoot string
	// Dir is where a plugin's hook runs when that is not PluginRoot: the plugin's real directory
	// when it was found through a symlink (recorded: runs/tui-plugins). "" is PluginRoot.
	Dir string
	// Local: the hook is of a plugin from the user's local plugins (~/.cursor/plugins/local),
	// which an interactive session loads (local.go).
	Local bool
	// User: the hook is the user's own (~/.cursor/hooks.json), not the project's.
	User bool
}

// Config is the project's hooks: the entries configured for each event, in
// order.
type Config struct {
	entries map[Event][]Entry
}

// Load reads the hooks of each plugin directory loaded with --plugin-dir
// (plugin.go), then <dir>/.cursor/hooks.json. No file is no hooks. A hook's
// command, failClosed, matcher and timeout are modeled; its loop_limit, the prompt type
// of hook and the team and enterprise sources are not. The user's own
// ~/.cursor/hooks.json is read after the project's, and a hook in both runs once per
// source (recorded: runs/hooks-all-matching-run-same-hook-two-sources).
//
// sr:docs https://cursor.com/docs/hooks#configuration
func Load(dir, home string, pluginDirs ...string) (Config, error) {
	return load(dir, home, false, false, pluginDirs)
}

// LoadStopOptIn is Load for a print-mode run with the stop opt-in (A10N_CURSOR_MOCK_STOP), which
// fires what the TUI fires at the end of a turn: which plugin hooks run is then that of the TUI
// (applyTUIPluginRules), but the user's local plugins are not loaded, as print mode does not.
func LoadStopOptIn(dir, home string, pluginDirs ...string) (Config, error) {
	return load(dir, home, false, true, pluginDirs)
}

// LoadInteractive is Load for a TUI session: the user's local plugins are loaded too, after those
// of --plugin-dir, and which plugin hooks run is that of the TUI (local.go).
func LoadInteractive(dir, home string, pluginDirs ...string) (Config, error) {
	return load(dir, home, true, true, pluginDirs)
}

func load(dir, home string, interactive, tuiRules bool, pluginDirs []string) (Config, error) {
	c := Config{entries: map[Event][]Entry{}}
	// a loaded plugin's hooks are listed, and started, before the project's: the one
	// recording (runs/plugin-hooks) logs the plugin's first
	for _, p := range pluginDirs {
		if err := c.addPlugin(dir, p); err != nil {
			return Config{}, err
		}
	}
	if interactive && home != "" {
		if err := c.addLocalPlugins(home); err != nil {
			return Config{}, err
		}
	}
	if err := c.addFile(filepath.Join(dir, ".cursor", "hooks.json"), Entry{}); err != nil {
		return Config{}, err
	}
	if home != "" {
		if err := c.addFile(filepath.Join(home, ".cursor", "hooks.json"), Entry{User: true}); err != nil {
			return Config{}, err
		}
	}
	if tuiRules {
		if err := c.applyTUIPluginRules(); err != nil {
			return Config{}, err
		}
	}
	return c, nil
}

// addFile adds the hooks of one hooks.json after those already configured, each
// with the source fields of from (its plugin, or the user's); a missing file adds none.
func (c Config) addFile(path string, from Entry) error {
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
				e := from
				e.Command, e.FailClosed, e.Matcher, e.Timeout = d.Command, d.FailClosed, d.Matcher, time.Duration(d.Timeout*float64(time.Second))
				c.entries[Event(name)] = append(c.entries[Event(name)], e)
			}
		}
	}
	return nil
}

// Entries are the hooks configured for the event, in order.
func (c Config) Entries(e Event) []Entry { return c.entries[e] }
