package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A tool call a script asks for that the mock does not implement fails the run,
// before it is played, naming the tool (adr/tool-calls-validated).
func TestAScriptCallTheMockDoesNotImplementFailsTheRun(t *testing.T) {
	call := func(name, input string) string {
		return "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"tool_use\",\"id\":\"x\",\"name\":\"" + name + "\",\"input\":" + input + "}]}}'\n"
	}
	for name, s := range map[string]struct{ script, want string }{
		"unknown tool":      {call("Teleport", `{}`), "Teleport"},
		"unknown parameter": {call("Bash", `{"command":"ls","sandbox_permissions":"x"}`), `unknown parameter "sandbox_permissions"`},
		"wrong type":        {call("Bash", `{"command":5}`), `"command"`},
	} {
		t.Run(name, func(t *testing.T) {
			r := execMock(t, scenario{Script: s.script, Prompt: "go"})
			assert.NotZero(t, r.Code)
			assert.Contains(t, r.Stderr, s.want)
		})
	}
}
