package hooks

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Settings mirrors the subset of Claude Code settings.json that configures hooks.
type Settings struct {
	Hooks map[EventName][]HookEntry `json:"hooks"`
}

// HookEntry is one matcher+handler group under an event name.
type HookEntry struct {
	Matcher string        `json:"matcher"`
	Hooks   []HandlerSpec `json:"hooks"`
}

// HandlerSpec describes a single hook handler (command or http).
type HandlerSpec struct {
	Type    string `json:"type"`
	Command string `json:"command,omitempty"`
	URL     string `json:"url,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
	Async   bool   `json:"async,omitempty"`
}

// LoadSettings reads hook settings from the standard Claude Code settings files,
// merging project and local settings (local wins by appending last). It also
// loads hooks declared by enabled plugins, resolving each plugin's marketplace
// from extraKnownMarketplaces and cloning/reusing it under the plugin cache.
// Missing files are silently ignored.
//
// pluginCacheDirOverride may be empty (falls back to CLAUDE_CODE_PLUGIN_CACHE_DIR
// env var, then /tmp/a10n-mock-plugins).
//
// a10n:docs https://code.claude.com/docs/en/settings
// a10n:docs https://code.claude.com/docs/en/plugin-marketplaces
func LoadSettings(projectDir, pluginCacheDirOverride string) (*Settings, error) {
	merged := &Settings{Hooks: make(map[EventName][]HookEntry)}
	enabledPlugins := make(map[string]bool)
	marketplaces := make(map[string]marketplaceCfg)

	paths := []string{
		filepath.Join(projectDir, ".claude", "settings.json"),
		filepath.Join(projectDir, ".claude", "settings.local.json"),
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		var s settingsWithPlugins
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		for evt, entries := range s.Hooks {
			merged.Hooks[evt] = append(merged.Hooks[evt], entries...)
		}
		// Local settings win: later files overwrite earlier per-key values.
		for key, enabled := range s.EnabledPlugins {
			enabledPlugins[key] = enabled
		}
		for name, cfg := range s.ExtraKnownMarketplaces {
			marketplaces[name] = cfg
		}
	}

	if len(enabledPlugins) > 0 {
		loadPluginHooks(merged, pluginCacheDir(pluginCacheDirOverride), enabledPlugins, marketplaces)
	}

	return merged, nil
}

// EntriesFor returns the handler entries configured for the given event,
// optionally filtered by matcher (e.g. tool name for PreToolUse).
// A blank or "*" matcher matches everything.
func (s *Settings) EntriesFor(event EventName, matcher string) []HandlerSpec {
	var out []HandlerSpec
	for _, entry := range s.Hooks[event] {
		if matchesEntry(entry.Matcher, matcher) {
			out = append(out, entry.Hooks...)
		}
	}
	return out
}

func matchesEntry(entryMatcher, value string) bool {
	if entryMatcher == "" || entryMatcher == "*" {
		return true
	}
	if value == "" {
		return true
	}
	return entryMatcher == value
}
