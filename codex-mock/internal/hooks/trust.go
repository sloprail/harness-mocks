package hooks

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Options are what a run was told that decides which hooks load and which of them run: the
// features it switched, the trust it was given, and the sandbox it asked for.
type Options struct {
	// Disabled is `--disable hooks`: no hook of any layer or plugin loads
	// (recorded: runs/disable-hooks).
	Disabled bool
	// BypassTrust is --dangerously-bypass-hook-trust: every enabled hook runs, trusted or not.
	BypassTrust bool
	// IgnoreUserConfig is --ignore-user-config: config.toml is not read, so its plugins, its
	// hook trust and its trusted projects are as if absent; the user layer's hooks.json is still
	// loaded (recorded: runs/ignore-user-config).
	IgnoreUserConfig bool
	// Sandbox is the sandbox the run asked for (-s, or danger-full-access for
	// --dangerously-bypass-approvals-and-sandbox), empty when it asked for none.
	Sandbox string
}

// SandboxTrustsProject is whether asking for this sandbox makes Codex trust the project it runs in:
// a workspace-write or danger-full-access run does, a read-only one or one that asked for none does
// not (recorded: runs/project-hooks-trust).
func SandboxTrustsProject(sandbox string) bool {
	return sandbox == "workspace-write" || sandbox == "danger-full-access"
}

// userConfig is what config.toml says of trust: the hash a hook was trusted at
// ([hooks.state."<key>"] trusted_hash) and the projects it trusts ([projects."<dir>"]
// trust_level = "trusted"). The file is otherwise not read here.
type userConfig struct {
	trustedHash map[string]string
	projects    map[string]bool
}

func readUserConfig(path string) (userConfig, error) {
	c := userConfig{trustedHash: map[string]string{}, projects: map[string]bool{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	var hookKey, project string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "["):
			hookKey, project = "", ""
			head := strings.Trim(line, "[] ")
			if k, ok := strings.CutPrefix(head, "hooks.state."); ok {
				hookKey = strings.Trim(k, `"`)
			} else if p, ok := strings.CutPrefix(head, "projects."); ok {
				project = strings.Trim(p, `"`)
			}
		case hookKey != "" && strings.HasPrefix(line, "trusted_hash"):
			_, v, _ := strings.Cut(line, "=")
			c.trustedHash[hookKey] = strings.Trim(strings.TrimSpace(v), `"`)
		case project != "" && strings.HasPrefix(line, "trust_level"):
			_, v, _ := strings.Cut(line, "=")
			c.projects[project] = strings.Trim(strings.TrimSpace(v), `"`) == "trusted"
		}
	}
	return c, nil
}

// ProjectTrustedByConfig is whether config.toml in codexHome lists dir as a trusted project.
func ProjectTrustedByConfig(codexHome, dir string) bool {
	c, err := readUserConfig(filepath.Join(codexHome, "config.toml"))
	return err == nil && c.projects[dir]
}

// PersistProjectTrust records dir as a trusted project in config.toml, as Codex does when a run
// asks for a sandbox that trusts it (recorded: runs/project-hooks-trust); a project already
// listed is left as it is.
func PersistProjectTrust(codexHome, dir string) error {
	if ProjectTrustedByConfig(codexHome, dir) {
		return nil
	}
	path := filepath.Join(codexHome, "config.toml")
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	text := string(old)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" {
		text += "\n"
	}
	text += "[projects.\"" + dir + "\"]\ntrust_level = \"trusted\"\n"
	return os.WriteFile(path, []byte(text), 0o644)
}

var camel = regexp.MustCompile(`([a-z0-9])([A-Z])`)

// snake is an event's name as the trust hash and key spell it: PreToolUse is pre_tool_use.
func snake(ev Event) string { return strings.ToLower(camel.ReplaceAllString(string(ev), "${1}_${2}")) }

// hookHash is the hash a handler is trusted at: the sha256 of the compact JSON, keys sorted, of the
// event, the group's matcher (left out when there is none) and the handler with its defaults filled in
// (the timeout is 600 when none is set), recorded against the hash Codex itself reports for hooks of
// each layer (runs/project-hooks-trust, runs/hook-trust-config).
func hookHash(ev Event, matcher string, h fileHandler) string {
	timeout := h.Timeout
	if timeout == 0 {
		timeout = int(DefaultTimeout.Seconds())
	}
	handler := map[string]any{"type": h.Type, "command": h.Command, "timeout": timeout, "async": h.Async}
	if h.StatusMessage != "" {
		handler["statusMessage"] = h.StatusMessage
	}
	identity := map[string]any{"event_name": snake(ev), "hooks": []any{handler}}
	if matcher != "" && ev != Stop && ev != UserPromptSubmit && ev != Interrupt { // an event that ignores a matcher hashes without one
		identity["matcher"] = matcher
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(identity) // maps encode with their keys sorted
	sum := sha256.Sum256(bytes.TrimRight(buf.Bytes(), "\n"))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// trust decides which handlers of a hooks file run.
type trust struct {
	bypass bool
	cfg    userConfig
}

// allows is whether the handler at key, hashed as hash, may run.
func (t trust) allows(key, hash string) bool { return t.bypass || t.cfg.trustedHash[key] == hash }
