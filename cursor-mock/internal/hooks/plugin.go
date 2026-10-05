package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// addPlugin adds to the hooks configured so far the hooks of the plugin
// loaded from pluginDir (relative to the workspace when not absolute): those
// of the hooks file its .cursor-plugin/plugin.json names, or hooks/hooks.json
// when it names none. They run like any other hook, alongside the project's
// (recorded: runs/plugin-hooks). A directory that is not loaded contributes
// nothing; a marketplace is Cursor's account side, not something the mock
// reads.
//
// sr:docs https://cursor.com/docs/reference/plugins#hooks-format
// sr:provides plugin-hooks/cursor
func (c Config) addPlugin(workspace, pluginDir string) error {
	if !filepath.IsAbs(pluginDir) {
		pluginDir = filepath.Join(workspace, pluginDir)
	}
	hooksFile := filepath.Join(pluginDir, "hooks", "hooks.json")
	if raw, err := os.ReadFile(filepath.Join(pluginDir, ".cursor-plugin", "plugin.json")); err == nil {
		var m struct {
			Hooks string `json:"hooks"`
		}
		if json.Unmarshal(raw, &m) == nil && m.Hooks != "" {
			hooksFile = filepath.Join(pluginDir, m.Hooks)
		}
	}
	return c.addFile(hooksFile)
}
