package main

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"

	"github.com/sloprail/harness-mocks/codex-mock/internal/unimplemented"
)

// refuseUnimplemented is the error for the first flag given that the mock does
// not implement, and for a -c override of a key it does not read.
func refuseUnimplemented(f *pflag.FlagSet) error {
	for _, name := range unimplemented.Flags {
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
