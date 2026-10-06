package hooks

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Args is the arguments of an exec-form command hook, held as one string so that
// a handler stays comparable (two copies of a hook are one hook).
type Args string

// UnmarshalJSON reads a JSON array of strings.
func (a *Args) UnmarshalJSON(b []byte) error {
	var list []string
	if err := json.Unmarshal(b, &list); err != nil {
		return err
	}
	*a = Args(strings.Join(list, "\x00"))
	return nil
}

// List is the arguments, nil for a hook that has none (a shell command line).
func (a Args) List() []string {
	if a == "" {
		return nil
	}
	return strings.Split(string(a), "\x00")
}

// UnimplementedError is a settings field the mock does not implement: the run is
// refused rather than the field ignored (adr/fail-fast-unimplemented).
type UnimplementedError struct{ What string }

func (e *UnimplementedError) Error() string {
	return "claude-mock: " + e.What + " is not implemented by the mock: it is refused rather than ignored"
}

// refuseUnmodelledFields refuses a hook's `shell` other than "bash", the one
// value recorded (snapshots/runs/hook-shell: it changed nothing there); any other
// stays refused until a recording covers it.
func refuseUnmodelledFields(evt EventName, entries []HookEntry) error {
	for _, e := range entries {
		for _, h := range e.Hooks {
			if h.Shell != "" && h.Shell != "bash" {
				return &UnimplementedError{What: fmt.Sprintf("the %s hook's shell %q (only \"bash\" is recorded)", evt, h.Shell)}
			}
		}
	}
	return nil
}

// Permissions is the part of a settings file's permissions the mock models: deny rules, each
// naming one Bash command exactly, as `Bash(<command>)`.
type Permissions struct {
	Deny []string `json:"deny,omitempty"`
}

// addDeny adds a file's deny rules. A rule of any other form (another tool, a prefix or wildcard
// pattern) is refused rather than ignored (adr/fail-fast-unimplemented): only the exact Bash
// command is recorded (snapshots/runs/permission-denied).
func (s *Settings) addDeny(p Permissions) error {
	for _, rule := range p.Deny {
		cmd, ok := strings.CutSuffix(strings.TrimPrefix(rule, "Bash("), ")")
		if !strings.HasPrefix(rule, "Bash(") || !ok || strings.ContainsAny(cmd, "*") || strings.HasSuffix(cmd, ":") {
			return &UnimplementedError{What: fmt.Sprintf("the permission rule %q (only Bash(<exact command>) is recorded)", rule)}
		}
		s.Deny = append(s.Deny, cmd)
	}
	return nil
}

// Denied is whether a deny rule refuses the call: a Bash call whose command is exactly a denied one.
// sr:provides noninteractive-run/claude
func (s *Settings) Denied(tool string, input json.RawMessage) (command string, denied bool) {
	var in struct {
		Command string `json:"command"`
	}
	if tool != "Bash" || json.Unmarshal(input, &in) != nil {
		return "", false
	}
	for _, d := range s.Deny {
		if d == in.Command {
			return in.Command, true
		}
	}
	return "", false
}

// accept takes a settings file's modelled parts: its deny rules. allowManagedHooksOnly, which blocks
// the hooks of plugins the managed settings do not force-enable, is refused wherever it is set
// (adr/fail-fast-unimplemented): the mock reads no managed settings, and the managed file is
// root-owned, so no run of it could be recorded.
// sr:docs https://code.claude.com/docs/en/hooks#hook-locations
func (s *Settings) accept(f settingsWithPlugins) error {
	if f.AllowManagedHooksOnly != nil {
		return &UnimplementedError{What: "allowManagedHooksOnly"}
	}
	return s.addDeny(f.Permissions)
}

// Configured is whether any handler is configured for the event.
func (s *Settings) Configured(event EventName) bool { return len(s.Hooks[event]) > 0 }
