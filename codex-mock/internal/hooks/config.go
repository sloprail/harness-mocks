// Package hooks is how codex-mock finds, matches, runs and reads Codex hooks.
package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
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
	PreCompact       Event = "PreCompact"
	PostCompact      Event = "PostCompact"
	SessionEnd       Event = "SessionEnd"
	Interrupt        Event = "Interrupt"
)

// Handler is one command hook.
type Handler struct {
	Command string
	// Timeout is in seconds; zero is Codex's default of 600.
	Timeout int
	// Async is a hook that runs in the background: the agent does not wait for it.
	Async bool
	// Env is what only this hook's command is told, KEY=VALUE (a plugin's hook gets its plugin's
	// root and data directory).
	Env []string
}

// Group is a matcher and the handlers that run when it matches.
type Group struct {
	Matcher  string
	Handlers []Handler
}

// Config is every hook Codex loads, by event: all of them run, a higher
// layer does not replace a lower one, and a hook listed in two files is two
// hooks, run twice.
type Config map[Event][]Group

type fileHandler struct {
	Type          string `json:"type"`
	Command       string `json:"command"`
	Timeout       int    `json:"timeout"`
	Async         bool   `json:"async"`
	StatusMessage string `json:"statusMessage"`
}

type fileGroup struct {
	Matcher string        `json:"matcher"`
	Hooks   []fileHandler `json:"hooks"`
}

// Load reads hooks.json from the user layer ($CODEX_HOME) and the project
// layer (<cwd>/.codex), in that order, then the hooks of the plugins the user
// layer's config.toml enables (plugins.go); a missing file is no hooks. Only
// command handlers are kept: Codex skips the other handler types it parses.
//
// A hook runs only when it is trusted: its hash is the trusted_hash of its key in config.toml,
// or the run bypasses trust; an untrusted hook is skipped without a word (recorded:
// runs/hook-trust-untrusted). The project layer loads only when the project is trusted, by
// config.toml or by the sandbox the run asked for (trust.go). With the hooks feature disabled,
// none loads.
//
// sr:docs https://developers.openai.com/codex/hooks#where-codex-looks-for-hooks
// sr:docs https://developers.openai.com/codex/hooks#review-and-trust-hooks
// sr:provides hooks-all-matching-run/codex
func Load(codexHome, cwd string, opts Options) (Config, error) {
	cfg := Config{}
	if opts.Disabled {
		return cfg, nil
	}
	uc := userConfig{trustedHash: map[string]string{}, projects: map[string]bool{}}
	if !opts.IgnoreUserConfig && codexHome != "" {
		var err error
		if uc, err = readUserConfig(filepath.Join(codexHome, "config.toml")); err != nil {
			return nil, err
		}
	}
	t := trust{bypass: opts.BypassTrust, cfg: uc}
	layers := []string{codexHome}
	if uc.projects[cwd] || SandboxTrustsProject(opts.Sandbox) {
		layers = append(layers, filepath.Join(cwd, ".codex"))
	}
	for _, dir := range layers {
		if dir == "" {
			continue
		}
		path := filepath.Join(dir, "hooks.json")
		key := path
		if real, err := filepath.EvalSymlinks(path); err == nil {
			key = real
		}
		if err := cfg.addFile(path, key, nil, t); err != nil {
			return nil, err
		}
	}
	if !opts.IgnoreUserConfig {
		if err := cfg.addPlugins(codexHome, t); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

// addFile adds the trusted hooks of one hooks.json, whose handlers' keys start with keyPrefix; env is
// what the commands are told beyond the run's. A missing file adds none.
func (cfg Config) addFile(path, keyPrefix string, env []string, t trust) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var f struct {
		Hooks map[Event][]fileGroup `json:"hooks"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	for ev, groups := range f.Hooks {
		for gi, g := range groups {
			out := Group{Matcher: g.Matcher}
			for hi, h := range g.Hooks {
				key := keyPrefix + ":" + snake(ev) + ":" + strconv.Itoa(gi) + ":" + strconv.Itoa(hi)
				if h.Type == "command" && t.allows(key, hookHash(ev, g.Matcher, h)) {
					out.Handlers = append(out.Handlers, Handler{Command: h.Command, Timeout: h.Timeout, Async: h.Async, Env: env})
				}
			}
			cfg[ev] = append(cfg[ev], out)
		}
	}
	return nil
}
