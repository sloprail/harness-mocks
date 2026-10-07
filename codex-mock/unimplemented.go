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
		if !f.Changed(name) {
			continue
		}
		if name == "enable" || name == "disable" { // implemented for one feature only
			values, _ := f.GetStringArray(name)
			for _, v := range values {
				if v != unimplemented.Feature {
					return fmt.Errorf("codex-mock: --%s %s is not implemented by the mock (only the %s feature is): it is refused rather than ignored", name, v, unimplemented.Feature)
				}
			}
			continue
		}
		return fmt.Errorf("codex-mock: --%s is not implemented by the mock: it is refused rather than ignored", name)
	}
	overrides, _ := f.GetStringArray("config")
	for _, o := range overrides {
		if !strings.HasPrefix(o, "agents.max_depth=") {
			return fmt.Errorf("codex-mock: -c %s is not implemented by the mock (only agents.max_depth is read): it is refused rather than ignored", o)
		}
	}
	return nil
}

// sandboxOf is the sandbox the run asked for: -s, or danger-full-access when it bypasses the sandbox
// (which wins over -s, as recorded); an unknown mode is an error as Codex's is. Empty when it asked for none.
func sandboxOf(f *pflag.FlagSet) (string, error) {
	if bypass, _ := f.GetBool("dangerously-bypass-approvals-and-sandbox"); bypass {
		return "danger-full-access", nil
	}
	mode, _ := f.GetString("sandbox")
	switch mode {
	case "", "read-only", "workspace-write", "danger-full-access":
		return mode, nil
	}
	return "", fmt.Errorf("invalid value '%s' for '--sandbox <SANDBOX_MODE>' [possible values: read-only, workspace-write, danger-full-access]", mode)
}

// hooksDisabled is whether --disable hooks was given. Both --enable hooks and --disable hooks in one run is
// refused: which of the two wins is not recorded.
func hooksDisabled(f *pflag.FlagSet) (bool, error) {
	off, _ := f.GetStringArray("disable")
	on, _ := f.GetStringArray("enable")
	if len(off) > 0 && len(on) > 0 {
		return false, fmt.Errorf("codex-mock: --enable hooks together with --disable hooks is not implemented by the mock: which wins is not recorded")
	}
	return len(off) > 0, nil
}
