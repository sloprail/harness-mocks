package hooks

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
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
	// PluginRoot and PluginData are set on a hook a plugin contributed: where
	// the plugin is installed and where its persistent data lives.
	PluginRoot string `json:"-"`
	PluginData string `json:"-"`
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
// sr:docs https://code.claude.com/docs/en/settings
// sr:docs https://code.claude.com/docs/en/plugin-marketplaces
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
			merged.Hooks[evt] = mergeEntries(merged.Hooks[evt], entries)
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

// EntriesFor returns the handler entries configured for the given event whose
// matcher selects subject (what the event filters on: see matcherSubject). An
// event with no subject, or an empty one, takes every entry: its matcher is
// ignored.
func (s *Settings) EntriesFor(event EventName, subject string) []HandlerSpec {
	var out []HandlerSpec
	for _, entry := range s.Hooks[event] {
		// sr:provides hook-matcher-filter/claude
		if subject == "" || corehooks.Select(corehooks.MatchExactOrRegexp, entry.Matcher, subject) {
			out = append(out, entry.Hooks...)
		}
	}
	return out
}

// matcherSubject is what an event's matcher filters on (docs, "Matcher
// patterns"): the tool name for a tool event, how the session started or why
// it ended, the agent type of a sub-agent, what triggered a compaction. Other
// events have none.
func matcherSubject(in Input) string {
	switch in.HookEventName {
	case EventPreToolUse, EventPostToolUse, EventPostToolUseFailure:
		return in.ToolName
	case EventSessionStart:
		return in.Source
	case EventSessionEnd:
		return in.Reason
	case EventSubagentStart, EventSubagentStop:
		return in.AgentType
	case EventPreCompact, EventPostCompact:
		return in.Trigger
	}
	return ""
}

// mergeEntries adds the entries of one more settings file to an event's: a
// handler already there under the same matcher, from another file, runs once
// (docs, Hook handler fields). A plugin's copy is not merged here, so it stays
// separate.
func mergeEntries(have, add []HookEntry) []HookEntry {
	for _, e := range add {
		var fresh []HandlerSpec
		for _, h := range e.Hooks {
			if !hasHandler(have, e.Matcher, h) {
				fresh = append(fresh, h)
			}
		}
		if len(fresh) > 0 {
			have = append(have, HookEntry{Matcher: e.Matcher, Hooks: fresh})
		}
	}
	return have
}

func hasHandler(entries []HookEntry, matcher string, h HandlerSpec) bool {
	for _, e := range entries {
		if e.Matcher != matcher {
			continue
		}
		for _, o := range e.Hooks {
			if o == h {
				return true
			}
		}
	}
	return false
}
