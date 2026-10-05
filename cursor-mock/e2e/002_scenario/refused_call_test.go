package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A tool call a script asks for that the mock does not implement ends the run,
// before it is played, naming the tool (adr/tool-calls-validated): the main
// agent's, and a sub-agent's too.
func TestAScriptCallTheMockDoesNotImplementFailsTheRun(t *testing.T) {
	call := func(name, input string) string {
		return `printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"x","name":"` + name + `","input":` + input + `}]}}'`
	}
	out, stderr, code := run(t, "#!/bin/sh\n"+call("Teleport", `{}`), "go")
	require.NotEqual(t, 0, code, out)
	assert.Contains(t, stderr, "Teleport")
	assert.NotContains(t, out, "tool_call")

	_, stderr, code = run(t, "#!/bin/sh\n"+call("Shell", `{"command":"ls","timeout":5}`), "go")
	require.NotEqual(t, 0, code)
	assert.Contains(t, stderr, `unknown parameter "timeout"`)

	sub := filepath.Join(t.TempDir(), "sub.sh")
	require.NoError(t, os.WriteFile(sub, []byte("#!/bin/sh\n"+call("Teleport", `{}`)), 0o755))
	_, stderr, code = run(t, "#!/bin/sh\n"+call("Task", `{"description":"d","prompt":"p","script":"`+sub+`"}`), "go")
	require.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "Teleport")
}
