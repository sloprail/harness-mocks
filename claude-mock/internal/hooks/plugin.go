package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// plugin.go resolves Claude Code plugin hooks the same way the real client does,
// mirroring the prod skills loader (internal/claude/skills_loader_plugin.go):
//
//  1. enabledPlugins in settings declares "<plugin>@<marketplace>": true.
//  2. extraKnownMarketplaces in settings declares each marketplace's source
//     (git / github / directory). A marketplace is identified by the "name" field
//     in its .claude-plugin/marketplace.json — NOT by the settings key.
//  3. A git/github marketplace is shallow-cloned into the plugin cache once and
//     reused on subsequent runs (the cache exists precisely so we don't re-pull
//     every invocation). A directory marketplace is read in place.
//  4. Each enabled plugin's hooks live at
//     <marketplaceRoot>/plugins/<plugin>/hooks/hooks.json; ${CLAUDE_PLUGIN_ROOT}
//     in every command expands to that plugin directory.
//
// There is NO fallback: a plugin whose marketplace is not declared in
// extraKnownMarketplaces does not resolve, exactly like the real client.
//
// sr:docs https://code.claude.com/docs/en/plugin-marketplaces
// sr:docs https://code.claude.com/docs/en/settings (extraKnownMarketplaces, enabledPlugins)

// pluginCacheDir returns the directory used to cache cloned marketplace repos.
//
// Resolution order (mirrors the CLAUDE_CODE_PLUGIN_CACHE_DIR env var):
//  1. explicit override passed by the caller (from --plugin-cache-dir)
//  2. CLAUDE_CODE_PLUGIN_CACHE_DIR env var
//  3. /tmp/a10n-mock-plugins — a fixed path so marketplaces cloned by one test
//     run are reused by the next instead of being re-cloned every time.
//
// sr:docs https://code.claude.com/docs/en/env-vars#environment-variables (CLAUDE_CODE_PLUGIN_CACHE_DIR)
func pluginCacheDir(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if v := os.Getenv("CLAUDE_CODE_PLUGIN_CACHE_DIR"); v != "" {
		return v
	}
	return "/tmp/a10n-mock-plugins"
}

// settingsWithPlugins extends the base hook Settings with the two Claude Code
// plugin fields. enabledPlugins maps "<plugin>@<marketplace>" → bool;
// extraKnownMarketplaces maps an arbitrary key → a marketplace config object.
//
// sr:docs https://code.claude.com/docs/en/settings (extraKnownMarketplaces, enabledPlugins)
type settingsWithPlugins struct {
	Settings
	EnabledPlugins         map[string]bool           `json:"enabledPlugins,omitempty"`
	ExtraKnownMarketplaces map[string]marketplaceCfg `json:"extraKnownMarketplaces,omitempty"`
}

// marketplaceCfg is one entry of extraKnownMarketplaces.
//
// sr:docs https://code.claude.com/docs/en/plugin-marketplaces (marketplace sources)
type marketplaceCfg struct {
	Source marketplaceSource `json:"source"`
}

// marketplaceSource describes where a marketplace lives. The Source field is the
// discriminator: "git"/"github" use URL/Repo, "directory" uses Path.
//
// sr:docs https://code.claude.com/docs/en/plugin-marketplaces (source types)
type marketplaceSource struct {
	Source string `json:"source"` // "git" | "github" | "directory"
	URL    string `json:"url,omitempty"`
	Repo   string `json:"repo,omitempty"`
	Path   string `json:"path,omitempty"`
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
		if root == "" {
			fmt.Fprintf(os.Stderr, "claude-mock: warn: marketplace %s not declared in extraKnownMarketplaces\n", mpName)
			continue
		}

		plugins := byMarketplace[mpName]
		sort.Strings(plugins)
		for _, pluginName := range plugins {
			// Prefer the manifest's OWN declared source path for this plugin (the general
			// case — a plugin can live anywhere relative to the marketplace root, e.g.
			// "./marketplace/plugins/foo" for an in-monorepo marketplace whose manifest sits
			// at the repo root). Fall back to the historical plugins/<name> layout only when
			// the manifest has no entry for this plugin (e.g. a hand-rolled test fixture).
			pluginDir, ok := manifest.pluginSourceDir(root, pluginName)
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
				dst.Hooks[evt] = append(dst.Hooks[evt], expandPluginRoot(entries, pluginDir)...)
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

// resolveMarketplace returns the local filesystem root AND parsed manifest of the
// marketplace whose .claude-plugin/marketplace.json "name" equals marketplaceName, scanning
// every entry in extraKnownMarketplaces. git/github sources are cloned-or-reused under
// cacheDir; directory sources are read in place. Returns ("", zero-value, nil) when no
// declared marketplace matches.
//
// sr:docs https://code.claude.com/docs/en/plugin-marketplaces (source types)
func resolveMarketplace(marketplaceName string, marketplaces map[string]marketplaceCfg, cacheDir string) (string, marketplaceManifest, error) {
	// Deterministic scan order over the declared marketplaces.
	keys := make([]string, 0, len(marketplaces))
	for k := range marketplaces {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		src := marketplaces[k].Source
		var candidate string

		switch src.Source {
		case "directory":
			if src.Path == "" {
				continue
			}
			candidate = src.Path

		case "git", "github":
			gitURL := src.URL
			if gitURL == "" {
				gitURL = src.Repo
			}
			if gitURL == "" {
				continue
			}
			clonePath := filepath.Join(cacheDir, marketplaceSlug(gitURL))
			if err := ensureCloned(gitURL, clonePath); err != nil {
				return "", marketplaceManifest{}, fmt.Errorf("clone %s: %w", gitURL, err)
			}
			candidate = clonePath

		default:
			continue
		}

		// Verify identity via the marketplace.json name, exactly like the client.
		manifest, err := readMarketplaceManifest(candidate)
		if err != nil {
			fmt.Fprintf(os.Stderr, "claude-mock: warn: marketplace.json at %s: %v\n", candidate, err)
			continue
		}
		if manifest.Name == marketplaceName {
			return candidate, manifest, nil
		}
	}
	return "", marketplaceManifest{}, nil
}

// marketplaceSlug derives a stable, filesystem-safe cache directory name from a
// git URL (e.g. git@github.com:a10n-build/a10n-marketplace.git → a10n-marketplace).
func marketplaceSlug(gitURL string) string {
	s := gitURL
	s = strings.TrimSuffix(s, ".git")
	if i := strings.LastIndexAny(s, "/:"); i >= 0 {
		s = s[i+1:]
	}
	if s == "" {
		s = "marketplace"
	}
	return s
}

// ensureCloned shallow-clones gitURL into targetPath when it is not already a git
// checkout. When the clone already exists it is REUSED as-is (no pull) — the
// cache is what lets repeated runs avoid network round-trips.
//
// sr:docs https://code.claude.com/docs/en/plugin-marketplaces (cached clones)
func ensureCloned(gitURL, targetPath string) error {
	if _, err := os.Stat(filepath.Join(targetPath, ".git")); err == nil {
		return nil // already cached — reuse without pulling
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "clone",
		"--depth", "1", "--single-branch", "--no-tags",
		gitURL, targetPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(targetPath) // drop a partial clone
		return fmt.Errorf("git clone: %w\n%s", err, out)
	}
	return nil
}

// marketplaceManifest is the subset of .claude-plugin/marketplace.json this mock needs:
// the marketplace's own identity, plus each declared plugin's SOURCE path — which is NOT
// necessarily "plugins/<name>" (e.g. an in-monorepo marketplace whose manifest sits at the
// repo root but whose plugins live under a subdirectory, such as "./marketplace/plugins/foo").
//
// sr:docs https://code.claude.com/docs/en/plugin-marketplaces (manifest schema)
type marketplaceManifest struct {
	Name    string                    `json:"name"`
	Plugins []marketplaceManifestItem `json:"plugins"`
}

type marketplaceManifestItem struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// pluginSourceDir returns the plugin's source path, relative-joined against the marketplace
// root the same way the real Claude Code client resolves it ("./plugins/foo" and "plugins/foo"
// both join naturally via filepath.Join). ok=false when the manifest declares no entry for
// pluginName — the caller falls back to the legacy plugins/<name> layout.
func (m marketplaceManifest) pluginSourceDir(root, pluginName string) (string, bool) {
	for _, p := range m.Plugins {
		if p.Name == pluginName && p.Source != "" {
			return filepath.Join(root, p.Source), true
		}
	}
	return "", false
}

// readMarketplaceManifest reads <root>/.claude-plugin/marketplace.json.
func readMarketplaceManifest(root string) (marketplaceManifest, error) {
	data, err := os.ReadFile(filepath.Join(root, ".claude-plugin", "marketplace.json"))
	if err != nil {
		return marketplaceManifest{}, err
	}
	var m marketplaceManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return marketplaceManifest{}, err
	}
	return m, nil
}

// expandPluginRoot replaces ${CLAUDE_PLUGIN_ROOT} in every command with the
// plugin's install directory, mirroring the real Claude Code runtime. Any other
// ${VAR} placeholder is preserved (in braced form) so the shell resolves it at
// hook-run time.
//
// sr:docs https://code.claude.com/docs/en/plugins#hooks
func expandPluginRoot(entries []HookEntry, pluginDir string) []HookEntry {
	out := make([]HookEntry, len(entries))
	for i, e := range entries {
		expanded := make([]HandlerSpec, len(e.Hooks))
		for j, h := range e.Hooks {
			h.Command = os.Expand(h.Command, func(key string) string {
				if key == "CLAUDE_PLUGIN_ROOT" {
					return pluginDir
				}
				return "${" + key + "}"
			})
			expanded[j] = h
		}
		out[i] = HookEntry{Matcher: e.Matcher, Hooks: expanded}
	}
	return out
}
