// Package procexec is how a mock starts its child processes.
package procexec

import (
	"sort"
	"strings"
)

// Env is the environment a child process of the harness starts with: inherited,
// with every key of ident set to ident's value, or removed when that value is
// empty. Replaced, never appended after: a mock run nested inside a live
// harness session inherits that session's identity, and its children must see
// this run's, not the outer one's.
//
// sr:capability subprocess-session-env
func Env(inherited []string, ident map[string]string) []string {
	env := make([]string, 0, len(inherited)+len(ident))
	for _, kv := range inherited {
		key, _, _ := strings.Cut(kv, "=")
		if _, owned := ident[key]; !owned {
			env = append(env, kv)
		}
	}
	keys := make([]string, 0, len(ident))
	for k := range ident {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if ident[k] != "" {
			env = append(env, k+"="+ident[k])
		}
	}
	return env
}
