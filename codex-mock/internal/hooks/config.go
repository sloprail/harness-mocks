// Package hooks is how codex-mock finds, matches, runs and reads Codex hooks.
package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Event is a hook event Codex names, as it appears in hooks.json and in the
// payload's hook_event_name.
type Event string

// The events codex-mock fires: those a `codex exec` run was recorded firing.
const (
	SessionStart     Event = "SessionStart"
	UserPromptSubmit Event = "UserPromptSubmit"
	PreToolUse       Event = "PreToolUse"
	PostToolUse      Event = "PostToolUse"
	Stop             Event = "Stop"
	SessionEnd       Event = "SessionEnd"
)

// Handler is one command hook.
type Handler struct {
	Command string
	// Timeout is in seconds; zero is Codex's default of 600.
	Timeout int
}

// Group is a matcher and the handlers that run when it matches.
type Group struct {
	Matcher  string
	Handlers []Handler
}

// Config is every hook Codex loads, by event: all of them run, a higher
// layer does not replace a lower one.
type Config map[Event][]Group

type fileGroup struct {
	Matcher string `json:"matcher"`
	Hooks   []struct {
		Type    string `json:"type"`
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	} `json:"hooks"`
}

// Load reads hooks.json from the user layer ($CODEX_HOME) and the project
// layer (<cwd>/.codex), in that order; a missing file is no hooks. Only
// command handlers are kept: Codex skips the other handler types it parses.
//
// sr:docs https://developers.openai.com/codex/hooks#where-codex-looks-for-hooks
func Load(codexHome, cwd string) (Config, error) {
	cfg := Config{}
	for _, dir := range []string{codexHome, filepath.Join(cwd, ".codex")} {
		if dir == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "hooks.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var f struct {
			Hooks map[Event][]fileGroup `json:"hooks"`
		}
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, err
		}
		for ev, groups := range f.Hooks {
			for _, g := range groups {
				out := Group{Matcher: g.Matcher}
				for _, h := range g.Hooks {
					if h.Type == "command" {
						out.Handlers = append(out.Handlers, Handler{Command: h.Command, Timeout: h.Timeout})
					}
				}
				cfg[ev] = append(cfg[ev], out)
			}
		}
	}
	return cfg, nil
}
