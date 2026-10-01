package hooks

import (
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
