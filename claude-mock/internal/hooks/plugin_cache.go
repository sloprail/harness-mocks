package hooks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
