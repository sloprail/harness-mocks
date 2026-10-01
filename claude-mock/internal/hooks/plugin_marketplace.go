package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

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
