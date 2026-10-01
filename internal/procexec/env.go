// Package procexec is how a mock starts its child processes.
package procexec

import (
	"sort"
	"strings"
)

// Env is the environment a child process of the harness starts with:
// inherited, with every key of ident set to ident's value (or removed when
// that value is empty), and every key of defaults set to its value only when
// inherited carries none. ident is replaced, never appended after: a mock run
// nested inside a live harness session inherits that session's identity, and
// its children must see this run's. defaults are what the launcher may declare
// instead (how the harness was started, say), so an inherited value wins.
//
// sr:capability subprocess-session-env
func Env(inherited []string, ident, defaults map[string]string) []string {
	env := make([]string, 0, len(inherited)+len(ident)+len(defaults))
	have := map[string]bool{}
	for _, kv := range inherited {
		key, val, _ := strings.Cut(kv, "=")
		if _, owned := ident[key]; owned {
			continue
		}
		if _, dflt := defaults[key]; dflt && val == "" {
			continue
		}
		have[key] = true
		env = append(env, kv)
	}
	for _, k := range sortedKeys(defaults) {
		if !have[k] && defaults[k] != "" {
			env = append(env, k+"="+defaults[k])
		}
	}
	for _, k := range sortedKeys(ident) {
		if ident[k] != "" {
			env = append(env, k+"="+ident[k])
		}
	}
	return env
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
