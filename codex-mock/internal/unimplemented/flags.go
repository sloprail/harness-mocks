// Package unimplemented is the flags of `codex exec` that the mock takes but implements none of: the one
// list the mock's refusal and the replay's check of a recording made with such a flag both read.
package unimplemented

// Flags are the long names. The mock registers them so its refusal can name them, and refuses them: a mock
// fails fast on what it does not implement (adr/fail-fast-unimplemented), because a silently ignored input
// lets a wrong recording or test pass (a `--enable multi_agent_v2` run replayed on the default mode, an
// output schema that shapes nothing).
var Flags = []string{"enable", "disable", "output-last-message", "output-schema", "thread-source",
	"profile", "color", "ignore-rules", "strict-config", "approve-for-me"}

// Short are the one-letter names some of them also have.
var Short = map[string]string{"profile": "p", "output-last-message": "o"}

// Feature is the one feature --enable and --disable are implemented for: hooks (recorded:
// runs/disable-hooks). The two flags are refused for any other.
const Feature = "hooks"

// Is reports whether the command-line word at words[i] (--name, or -x) names one of the flags the
// mock refuses: --enable and --disable only when the feature they name is not Feature.
func Is(words []string, i int) bool {
	word := words[i]
	for _, name := range Flags {
		if word == "--"+name || (Short[name] != "" && word == "-"+Short[name]) {
			if name == "enable" || name == "disable" {
				return i+1 >= len(words) || words[i+1] != Feature
			}
			return true
		}
	}
	return false
}
