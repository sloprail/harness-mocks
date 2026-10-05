package main

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
)

// refuseFeatureSwitches refuses --enable / --disable: a feature switch the mock
// implements none of. Running as if it were on would pass for the real thing (a
// `--enable multi_agent_v2` recording replayed on the default mode), so it is
// refused rather than accepted with no effect.
func refuseFeatureSwitches(f *pflag.FlagSet) error {
	for _, name := range []string{"enable", "disable"} {
		if v, _ := f.GetStringArray(name); len(v) > 0 {
			return fmt.Errorf("codex-mock: --%s %s: the mock implements no feature switch", name, strings.Join(v, ","))
		}
	}
	return nil
}
