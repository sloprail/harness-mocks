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
// merging project and local settings (local wins on conflict).
// Missing files are silently ignored.
func LoadSettings(projectDir string) (*Settings, error) {
	merged := &Settings{Hooks: make(map[EventName][]HookEntry)}
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
		var s Settings
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		for evt, entries := range s.Hooks {
			merged.Hooks[evt] = append(merged.Hooks[evt], entries...)
		}
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
