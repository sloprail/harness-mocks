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
	Deny  []string                  `json:"-"` // the Bash commands deny rules refuse (Permissions)
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
	// Args: the program run directly, no shell (runs/hook-args); Shell: ignored (runs/hook-shell).
	Args    Args   `json:"args,omitempty"`
	Shell   string `json:"shell,omitempty"`
	URL     string `json:"url,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
	Async   bool   `json:"async,omitempty"`
	// PluginRoot and PluginData are set on a hook a plugin contributed: where
	// the plugin is installed and where its persistent data lives.
	PluginRoot string `json:"-"`
	PluginData string `json:"-"`
}

// LoadSettings reads hook settings and deny rules from the standard Claude Code settings files
// (project and local merged, missing ones ignored) and the hooks of enabled plugins, resolving
// each plugin's marketplace from extraKnownMarketplaces under the plugin cache
// (pluginCacheDirOverride, else CLAUDE_CODE_PLUGIN_CACHE_DIR, else /tmp/a10n-mock-plugins).
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
		if err := merged.addDeny(s.Permissions); err != nil {
			return nil, err
		}
		for evt, entries := range s.Hooks {
			if err := refuseUnmodelledFields(evt, entries); err != nil {
				return nil, err
			}
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
// event with no subject takes every entry: its matcher is ignored. An event
// that has one, even an empty one (a sub-agent of no type), is filtered by it.
func (s *Settings) EntriesFor(event EventName, subject string) []HandlerSpec {
	var out []HandlerSpec
	for _, entry := range s.Hooks[event] {
		// sr:provides hook-matcher-filter/claude
		if !hasSubject(event) || corehooks.Select(matcherStyle(event), entry.Matcher, subject) {
			out = append(out, entry.Hooks...)
		}
	}
	return out
}

// hasSubject reports whether an event's matcher filters on something (docs,
// Matcher patterns); a matcher given to any other event is silently ignored.
func hasSubject(event EventName) bool {
	switch event {
	case EventPreToolUse, EventPostToolUse, EventPostToolUseFailure, EventSessionStart, EventSessionEnd,
		EventSubagentStart, EventSubagentStop, EventPreCompact, EventPostCompact, EventStopFailure:
		return true
	}
	return false
}

// matcherStyle is how an event's matcher is read: StopFailure has the
// narrower exact-match set (docs, Matcher patterns), every other event the
// usual one.
func matcherStyle(event EventName) corehooks.MatcherStyle {
	if event == EventStopFailure {
		return corehooks.MatchExactOrRegexpNarrow
	}
	return corehooks.MatchExactOrRegexp
}

// matcherSubject is what an event's matcher filters on (docs, "Matcher
// patterns"): the tool name for a tool event, how the session started or why
// it ended, the agent type of a sub-agent, what triggered a compaction, the
// error type of a failed turn (StopFailure). Other events have none.
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
	case EventStopFailure:
		return in.Error
	}
	return ""
}
