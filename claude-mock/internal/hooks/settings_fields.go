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
