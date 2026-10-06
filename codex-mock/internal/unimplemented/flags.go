// Package unimplemented is the flags of `codex exec` that the mock takes but implements none of: the one
// list the mock's refusal and the replay's check of a recording made with such a flag both read.
package unimplemented

// Flags are the long names. The mock registers them so its refusal can name them, and refuses them: a mock
// fails fast on what it does not implement (adr/fail-fast-unimplemented), because a silently ignored input
// lets a wrong recording or test pass (a `--enable multi_agent_v2` run replayed on the default mode, an
// output schema that shapes nothing).
var Flags = []string{"enable", "disable", "output-last-message", "output-schema", "thread-source",
	"sandbox", "profile", "color", "ignore-user-config", "ignore-rules", "strict-config", "approve-for-me"}

// Short are the one-letter names some of them also have.
var Short = map[string]string{"sandbox": "s", "profile": "p", "output-last-message": "o"}

// Is reports whether a command-line word (--name, or -x) names one of the flags.
func Is(word string) bool {
	for _, name := range Flags {
		if word == "--"+name || (Short[name] != "" && word == "-"+Short[name]) {
			return true
		}
	}
	return false
}
