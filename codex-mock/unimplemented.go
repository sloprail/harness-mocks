package main

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
)

// unimplemented are the flags of `codex exec` that the mock takes but implements
// none of. They are registered so the refusal can name them, and are refused: a
// mock fails fast on what it does not implement (adr/fail-fast-unimplemented),
// because a silently ignored input lets a wrong recording or test pass (a
// `--enable multi_agent_v2` run replayed on the default mode, an output schema
// that shapes nothing).
var unimplemented = []string{"enable", "disable", "output-last-message", "output-schema", "thread-source",
	"sandbox", "profile", "color", "ignore-user-config", "ignore-rules", "strict-config", "approve-for-me"}

// refuseUnimplemented is the error for the first flag given that the mock does
// not implement, and for a -c override of a key it does not read.
func refuseUnimplemented(f *pflag.FlagSet) error {
	for _, name := range unimplemented {
		if f.Changed(name) {
			return fmt.Errorf("codex-mock: --%s is not implemented by the mock: it is refused rather than ignored", name)
		}
	}
	overrides, _ := f.GetStringArray("config")
	for _, o := range overrides {
		if !strings.HasPrefix(o, "agents.max_depth=") {
			return fmt.Errorf("codex-mock: -c %s is not implemented by the mock (only agents.max_depth is read): it is refused rather than ignored", o)
		}
	}
	return nil
}
