package replay

import (
	"fmt"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// mockCall is Claude's name for a unified call; the input is copied, as the
// spawn's script parameter is added to it. The spawn's message is the prompt
// the input already carries.
func mockCall(c core.Call) scriptCall {
	name := "Bash"
	switch c.Tool {
	case core.ToolSpawn:
		name = "Agent"
	case toolRead, toolWrite, toolEdit, toolGlob:
		for claude, unified := range fileTools {
			if unified == c.Tool {
				name = claude
			}
		}
	case toolReply:
		return scriptCall{Reply: c.Input["text"].(string)}
	}
	in := make(map[string]any, len(c.Input))
	for k, v := range c.Input {
		if k != "message" && k != gateKey {
			in[k] = v
		}
	}
	return scriptCall{Text: c.Said, Name: name, Input: in, Gated: c.Input[gateKey] == true}
}

// script is the mock script that makes the given calls, one per turn, then
// answers with final. The calls are inside it, so a sub-agent's script is a
// file of its own. The mock runs the script once per tool call and the session
// file holds the results so far, so the script's n-th run makes the n-th call.
// Call ids are unique across the run's scripts (tag), as the real ones are.
func script(tag string, calls []scriptCall, final, extra string, skip int) string {
	lines := make([]string, 0, len(calls)+1)
	for i, c := range calls {
		lines = append(lines, callLine(fmt.Sprintf("toolu_%s%d", tag, i), c))
	}
	lines = append(lines, finalLines(final, extra))
	return fmt.Sprintf(`#!/bin/sh
n=$(grep -c '"type":"tool_result"' "$A10N_MOCK_SESSION_FILE")
f=$(grep -c '"content":"Stop hook feedback:' "$A10N_MOCK_SESSION_FILE")
t=$(grep -c '"turnOrigin":"task_notification"' "$A10N_MOCK_SESSION_FILE")
n=$((n+f+t))
sed -n "$((n+1-%d))p" <<'CALLS_EOF' | tr '\001' '\n'
%s
CALLS_EOF
`, skip, strings.Join(lines, "\n"))
}
