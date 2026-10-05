package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sloprail/harness-mocks/claude-mock/e2etest"
)

// The mock fails fast on what it does not implement (adr/fail-fast-unimplemented):
// an output format other than stream-json, and --agent (which would put
// agent_type on main-thread hook payloads), are refused with an error naming
// them, and the script never runs.
// sr:proves noninteractive-run/claude
// sr:proves hook-common-payload/claude
func TestT001_09_UnimplementedInputsAreRefused(t *testing.T) {
	const script = "#!/bin/sh\necho RAN\n"
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"text":  {[]string{"--output-format", "text"}, "--output-format text is not implemented by the mock"},
		"json":  {[]string{"--output-format", "json"}, "--output-format json is not implemented by the mock"},
		"agent": {[]string{"--agent", "reviewer"}, "unknown flag: --agent"},
	} {
		t.Run(name, func(t *testing.T) {
			out, code := e2etest.RunWithScript(t, script, append(tc.args, "--session-id", "s-1", "-p", "go")...)
			assert.NotZero(t, code, out)
			assert.Contains(t, out, tc.want)
			assert.NotContains(t, out, "RAN")
		})
	}
}
