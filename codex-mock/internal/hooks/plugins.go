package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// pluginTables is what the user layer's config.toml says of plugins: the
// marketplaces it declares ([marketplaces.<name>], with the directory of a
// local one as source) and the plugins it enables or disables
// ([plugins."<plugin>@<marketplace>"], enabled = true|false). Only these two
// tables are read; the file is otherwise not interpreted.
type pluginTables struct {
	marketplaces map[string]string
	plugins      []pluginSetting // in file order
}

type pluginSetting struct {
	name, marketplace string
	enabled           bool
}

// readPluginTables parses the two tables out of config.toml; a missing file
// declares nothing.
func readPluginTables(path string) (pluginTables, error) {
	t := pluginTables{marketplaces: map[string]string{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return t, nil
	}
	if err != nil {
		return t, err
	}
	var market string // the [marketplaces.<name>] table being read
	plugin := -1      // index in t.plugins of the [plugins."..."] table being read
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "["):
			market, plugin = "", -1
			head := strings.Trim(line, "[] ")
			if name, ok := strings.CutPrefix(head, "marketplaces."); ok {
				market = strings.Trim(name, `"`)
			} else if sel, ok := strings.CutPrefix(head, "plugins."); ok {
				name, mk, _ := strings.Cut(strings.Trim(sel, `"`), "@")
				t.plugins = append(t.plugins, pluginSetting{name: name, marketplace: mk})
				plugin = len(t.plugins) - 1
			}
		case market != "" && strings.HasPrefix(line, "source "), market != "" && strings.HasPrefix(line, "source="):
			_, v, _ := strings.Cut(line, "=")
			t.marketplaces[market] = strings.Trim(strings.TrimSpace(v), `"`)
		case plugin >= 0 && strings.HasPrefix(line, "enabled"):
			_, v, _ := strings.Cut(line, "=")
			t.plugins[plugin].enabled = strings.TrimSpace(v) == "true"
		}
	}
	return t, nil
}

// addPlugins adds, after the hooks the layers hold, the hooks of every plugin
// that contributes: one the config.toml enables, from a marketplace it
// declares (a plugin of another marketplace, or disabled, adds none). A plugin
// is found through its marketplace's .agents/plugins/marketplace.json, and its
// hooks are its hooks/hooks.json, which run like any other hook. A declared
// marketplace is a directory on disk.
//
// sr:docs https://developers.openai.com/codex/hooks#plugin-bundled-hooks
// sr:docs https://developers.openai.com/plugins/build/plugins#add-a-marketplace-from-the-cli
// sr:provides plugin-hooks/codex
func (cfg Config) addPlugins(codexHome string) error {
	if codexHome == "" {
		return nil
	}
	t, err := readPluginTables(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		return err
	}
	for _, p := range t.plugins {
		root, declared := t.marketplaces[p.marketplace]
		if !corehooks.PluginContributes(p.enabled, declared) {
			continue
		}
		dir := pluginDir(root, p.name)
		if dir == "" {
			continue
		}
		if err := cfg.addFile(filepath.Join(dir, "hooks", "hooks.json")); err != nil {
			return err
		}
	}
	return nil
}

// pluginDir is the directory of the named plugin in the marketplace at root,
// or "" when the marketplace does not list it.
func pluginDir(root, name string) string {
	data, err := os.ReadFile(filepath.Join(root, ".agents", "plugins", "marketplace.json"))
	if err != nil {
		return ""
	}
	var m struct {
		Plugins []struct {
			Name   string `json:"name"`
			Source struct {
				Path string `json:"path"`
			} `json:"source"`
		} `json:"plugins"`
	}
	if json.Unmarshal(data, &m) != nil {
		return ""
	}
	for _, p := range m.Plugins {
		if p.Name == name && p.Source.Path != "" {
			return filepath.Join(root, p.Source.Path)
		}
	}
	return ""
}
