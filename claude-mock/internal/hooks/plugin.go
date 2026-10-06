package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// settingsWithPlugins extends the base hook Settings with the two Claude Code
// plugin fields. enabledPlugins maps "<plugin>@<marketplace>" → bool;
// extraKnownMarketplaces maps an arbitrary key → a marketplace config object.
//
// sr:docs https://code.claude.com/docs/en/settings (extraKnownMarketplaces, enabledPlugins)
type settingsWithPlugins struct {
	Settings
	Permissions            Permissions               `json:"permissions"`
	EnabledPlugins         map[string]bool           `json:"enabledPlugins,omitempty"`
	ExtraKnownMarketplaces map[string]marketplaceCfg `json:"extraKnownMarketplaces,omitempty"`
}

// pluginHooks is the schema of a plugin's hooks/hooks.json file. Each value is a
// list of HookEntry objects identical to those in settings.json.
//
// sr:docs https://code.claude.com/docs/en/plugins#hooks
type pluginHooks struct {
	Hooks map[EventName][]HookEntry `json:"hooks"`
}

// loadPluginHooks resolves every enabled plugin via its marketplace and appends
// the plugin's hooks (with ${CLAUDE_PLUGIN_ROOT} expanded) to dst.Hooks.
//
// enabledPlugins / extraKnownMarketplaces come straight from the merged settings.
// cacheDir is where git marketplaces are cloned (and reused). Resolution failures
// for one plugin are logged and skipped — they never abort the others.
//
// sr:docs https://code.claude.com/docs/en/plugin-marketplaces
func loadPluginHooks(dst *Settings, cacheDir string, enabledPlugins map[string]bool, marketplaces map[string]marketplaceCfg) {
	byMarketplace := groupEnabledByMarketplace(enabledPlugins)
	if len(byMarketplace) == 0 {
		return
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "claude-mock: warn: plugin cache dir: %v\n", err)
		return
	}

	// Deterministic order across marketplaces and plugins for stable output.
	mpNames := make([]string, 0, len(byMarketplace))
	for name := range byMarketplace {
		mpNames = append(mpNames, name)
	}
	sort.Strings(mpNames)

	for _, mpName := range mpNames {
		root, manifest, err := resolveMarketplace(mpName, marketplaces, cacheDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "claude-mock: warn: marketplace %s: %v\n", mpName, err)
			continue
		}
		declared := root != ""
		if !declared {
			fmt.Fprintf(os.Stderr, "claude-mock: warn: marketplace %s not declared in extraKnownMarketplaces\n", mpName)
		}

		plugins := byMarketplace[mpName]
		sort.Strings(plugins)
		for _, pluginName := range plugins {
			// sr:provides plugin-hooks/claude
			if !corehooks.PluginContributes(enabledPlugins[pluginName+"@"+mpName], declared) {
				continue
			}
			// Prefer the manifest's OWN declared source path for this plugin (the general
			// case — a plugin can live anywhere relative to the marketplace root, e.g.
			// "./marketplace/plugins/foo" for an in-monorepo marketplace whose manifest sits
			// at the repo root). Fall back to the historical plugins/<name> layout only when
			// the manifest has no entry for this plugin (e.g. a hand-rolled test fixture).
			pluginDir, ok, err := manifest.pluginSourceDir(root, pluginName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "claude-mock: error: plugin %s@%s: %v\n", pluginName, mpName, err)
				continue
			}
			if !ok {
				pluginDir = filepath.Join(root, "plugins", pluginName)
			}
			hooksFile := filepath.Join(pluginDir, "hooks", "hooks.json")
			data, err := os.ReadFile(hooksFile)
			if err != nil {
				if !os.IsNotExist(err) {
					fmt.Fprintf(os.Stderr, "claude-mock: warn: plugin %s@%s hooks: %v\n", pluginName, mpName, err)
				}
				continue
			}
			var ph pluginHooks
			if err := json.Unmarshal(data, &ph); err != nil {
				fmt.Fprintf(os.Stderr, "claude-mock: warn: plugin %s@%s hooks.json: %v\n", pluginName, mpName, err)
				continue
			}
			for evt, entries := range ph.Hooks {
				dst.Hooks[evt] = append(dst.Hooks[evt], expandPluginRoot(entries, pluginDir, cacheDir)...)
			}
		}
	}
}

// groupEnabledByMarketplace turns {"<plugin>@<marketplace>": true} into
// {marketplace: [plugin...]} for the enabled entries only. The split is on the
// LAST '@' so plugin names containing '@' still resolve.
func groupEnabledByMarketplace(enabledPlugins map[string]bool) map[string][]string {
	out := make(map[string][]string)
	for key, enabled := range enabledPlugins {
		if !enabled {
			continue
		}
		plugin, marketplace, err := splitPluginKey(key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "claude-mock: warn: %v\n", err)
			continue
		}
		out[marketplace] = append(out[marketplace], plugin)
	}
	return out
}

// splitPluginKey splits "<pluginName>@<marketplace>" into its two parts on the
// last '@'.
func splitPluginKey(key string) (pluginName, marketplace string, err error) {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '@' {
			return key[:i], key[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("invalid plugin key %q (expected <name>@<marketplace>)", key)
}
