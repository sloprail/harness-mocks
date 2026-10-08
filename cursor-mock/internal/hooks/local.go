package hooks

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// addLocalPlugins loads the plugins the TUI finds in the user's <home>/.cursor/plugins/local: each
// directory in it that holds a .cursor-plugin/plugin.json, and each symlink there whose target
// stays inside that directory; a name that starts with a dot is skipped, and a symlink to a plugin
// elsewhere is not loaded (recorded: runs/tui-plugins, whose plugins p2 (a link to a directory
// outside) never ran a hook, and p4 (a link into a dot-named directory inside) did). A plugin's
// hooks run in its real directory and are told the path they were found at as CURSOR_PLUGIN_ROOT.
// A directory with no manifest is not a Cursor plugin, and is skipped (not recorded).
//
// The hooks file is the one plugin.go reads. They are listed in the order of the directory.
//
// sr:docs https://cursor.com/docs/plugins#what-plugins-contain
func (c Config) addLocalPlugins(home string) error {
	local := filepath.Join(home, ".cursor", "plugins", "local")
	entries, err := os.ReadDir(local)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	inside, err := filepath.EvalSymlinks(local)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(local, e.Name())
		real, err := filepath.EvalSymlinks(path)
		if err != nil || !strings.HasPrefix(real, inside+string(filepath.Separator)) {
			continue
		}
		if fi, err := os.Stat(real); err != nil || !fi.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(real, ".cursor-plugin", "plugin.json")); err != nil {
			continue
		}
		if err := c.addPluginAt(real, Entry{PluginRoot: path, Dir: real, Local: true}); err != nil {
			return err
		}
	}
	return nil
}

// applyTUIPluginRules drops the plugin hooks a TUI session, and a print-mode run with the stop
// opt-in (A10N_CURSOR_MOCK_STOP), does not run (recorded:
// runs/tui-plugins, runs/tui-plugins-event-gating):
//
//   - a local plugin's sessionStart hook: the local plugins are loaded in the background after the
//     session has started, so sessionStart is over by then (and the first prompt, if typed at once,
//     runs without them: the mock loads them before it, as a prompt typed later finds them);
//   - a plugin's stop hook: it never ran, though the project's did;
//   - a plugin's beforeSubmitPrompt and afterAgentResponse hooks, unless the project's hooks.json
//     configures a hook for the same event: with none, the plugin's did not run either. When
//     only the user's hooks.json configures one, what the TUI does was not recorded, and the
//     mock refuses it.
func (c Config) applyTUIPluginRules() error {
	drop := func(e Event, gone func(Entry) bool) {
		kept := c.entries[e][:0:0]
		for _, en := range c.entries[e] {
			if !gone(en) {
				kept = append(kept, en)
			}
		}
		c.entries[e] = kept
	}
	isPlugin := func(en Entry) bool { return en.PluginRoot != "" }
	drop(SessionStart, func(en Entry) bool { return en.Local })
	drop(Stop, isPlugin)
	for _, e := range []Event{BeforeSubmitPrompt, AfterAgentResponse} {
		project, user, plugin := false, false, false
		for _, en := range c.entries[e] {
			switch {
			case isPlugin(en):
				plugin = true
			case en.User:
				user = true
			default:
				project = true
			}
		}
		switch {
		case !plugin || project:
		case user:
			return fmt.Errorf("cursor-mock: a plugin's %s hook beside the user's, with none in the project's hooks.json, is not modeled: only the project's hooks.json was recorded as the source that makes a TUI session run it (runs/tui-plugins)", e)
		default:
			drop(e, isPlugin)
		}
	}
	return nil
}
