package hooks

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
)

// localVersion is the version Codex gives a plugin whose manifest names none (recorded: runs/plugin-install).
const localVersion = "local"

// installPlugin installs the plugin at src, from marketplace into codexHome's cache the way `codex plugin
// add` does (recorded: runs/plugin-install), and returns where it is: plugins/cache/<marketplace>/<plugin>/<version>.
// The version is the manifest's (.codex-plugin/plugin.json), "local" when it has none. The whole
// directory is copied, dotfiles too, but not a symbolic link. Installing a version replaces the plugin's
// cache whole, so no earlier version stays; a plugin already installed at its version is left as it is,
// as codex leaves it until it is added again.
func installPlugin(codexHome, marketplace, name, src string) (string, error) {
	version := localVersion
	if data, err := os.ReadFile(filepath.Join(src, ".codex-plugin", "plugin.json")); err == nil {
		var m struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(data, &m) == nil && m.Version != "" {
			version = m.Version
		}
	}
	plugin := filepath.Join(codexHome, "plugins", "cache", marketplace, name)
	dest := filepath.Join(plugin, version)
	if _, err := os.Stat(dest); err == nil {
		return dest, nil
	}
	if err := os.RemoveAll(plugin); err != nil {
		return "", err
	}
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dest, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case !d.Type().IsRegular(): // a symbolic link is not copied
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
	return dest, err
}
