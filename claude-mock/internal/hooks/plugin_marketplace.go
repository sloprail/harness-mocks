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
	Name   string       `json:"name"`
	Source pluginSource `json:"source"`
}

// pluginSource is a plugin entry's "source": either a string (a path relative to the
// marketplace root, e.g. "./plugins/foo") or an object whose "source" field names the kind
// ("github", "git", "url", "npm", "git-subdir"), as real Claude Code accepts.
//
// sr:docs https://code.claude.com/docs/en/plugin-marketplaces (plugin sources)
type pluginSource struct {
	Path   string // string form: the relative path. Object form: the object's "path".
	Kind   string // object form only: the "source" discriminator; "" for the string form
	URL    string
	Ref    string
	isObj  bool
	hasSrc bool
}

type pluginSourceObject struct {
	Source string `json:"source"`
	URL    string `json:"url,omitempty"`
	Path   string `json:"path,omitempty"`
	Ref    string `json:"ref,omitempty"`
}

func (s *pluginSource) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = pluginSource{Path: str, hasSrc: str != ""}
		return nil
	}
	var o pluginSourceObject
	if err := json.Unmarshal(data, &o); err != nil {
		return fmt.Errorf("plugin source must be a string or an object: %w", err)
	}
	*s = pluginSource{Path: o.Path, Kind: o.Source, URL: o.URL, Ref: o.Ref, isObj: true, hasSrc: true}
	return nil
}

func (s pluginSource) MarshalJSON() ([]byte, error) {
	if !s.isObj {
		return json.Marshal(s.Path)
	}
	return json.Marshal(pluginSourceObject{Source: s.Kind, URL: s.URL, Path: s.Path, Ref: s.Ref})
}

// pluginSourceDir returns the plugin's source path, relative-joined against the marketplace
// root the same way the real Claude Code client resolves it ("./plugins/foo" and "plugins/foo"
// both join naturally via filepath.Join). ok=false (nil error) when the manifest declares no
// entry for pluginName — the caller falls back to the legacy plugins/<name> layout.
//
// Object sources: "git-subdir" resolves to its "path" relative to the marketplace root. This
// is an APPROXIMATION: real Claude Code fetches the named repo (url, at ref) and takes the
// subdirectory; the mock assumes the marketplace IS that repo, so it never fetches. Other
// known kinds (github, git, url, npm) point at a separate checkout the mock cannot resolve,
// and unknown kinds are unrecognised; both return an error rather than being skipped.
func (m marketplaceManifest) pluginSourceDir(root, pluginName string) (string, bool, error) {
	for _, p := range m.Plugins {
		if p.Name != pluginName || !p.Source.hasSrc {
			continue
		}
		src := p.Source
		if !src.isObj {
			return filepath.Join(root, src.Path), true, nil
		}
		switch src.Kind {
		case "git-subdir":
			if src.Path == "" {
				return "", false, fmt.Errorf("plugin %q: git-subdir source has no \"path\"", pluginName)
			}
			return filepath.Join(root, src.Path), true, nil
		case "github", "git", "url", "npm":
			return "", false, fmt.Errorf("plugin %q: %q source needs a separate checkout, which the mock does not resolve (use a relative-path or git-subdir source)", pluginName, src.Kind)
		default:
			return "", false, fmt.Errorf("plugin %q: unknown source kind %q (known: git-subdir, github, git, url, npm)", pluginName, src.Kind)
		}
	}
	return "", false, nil
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
