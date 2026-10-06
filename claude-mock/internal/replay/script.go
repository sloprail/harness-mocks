package replay

import (
	"fmt"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
	"github.com/sloprail/harness-mocks/internal/scenario"
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
		silent, _ := c.Input["silent"].(bool)
		return scriptCall{Reply: c.Input["text"].(string), Early: c.SaidBefore, Silent: silent}
	case toolSend:
		return scriptCall{Text: c.Said, Name: "SendMessage", Input: copyInput(c.Input)}
	case core.ToolCompact:
		return scriptCall{Name: "compact", Input: c.Input, Control: true}
	}
	if strings.HasPrefix(c.Tool, toolPrefix) {
		name = strings.TrimPrefix(c.Tool, toolPrefix)
	}
	in := make(map[string]any, len(c.Input))
	for k, v := range c.Input {
		if k != "message" && k != gateKey {
			in[k] = v
		}
	}
	return scriptCall{Text: c.Said, Early: c.SaidBefore, Name: name, Input: in, Gated: c.Input[gateKey] == true, More: c.More}
}

// script is the mock script that makes the given calls, one per turn, then
// answers with final. The calls are inside it, so a sub-agent's script is a
// file of its own. The mock runs the script once per tool call and the session
// file holds the results so far, so the script's n-th run makes the n-th call.
// Call ids are unique across the run's scripts (tag), as the real ones are.
func script(tag string, calls []scriptCall, final, extra string, skip int, finalGate scenario.Gate) string {
	lines := make([]string, 0, len(calls)+1)
	for i := 0; i < len(calls); i++ {
		line, extra := callLine(fmt.Sprintf("toolu_%s%d", tag, i), calls[i]), 0
		for calls[i].More && i+1 < len(calls) { // a message of several calls is one run's output, each answered in turn
			i++
			line, extra = line+"\x01"+callLine(fmt.Sprintf("toolu_%s%d", tag, i), calls[i]), extra+1
		}
		lines = append(lines, line)
		for ; extra > 0; extra-- {
			lines = append(lines, "") // the lines the script's index skips: each of the message's results counts
		}
	}
	lines = append(lines, gateLine(finalGate)+finalLines(final, extra))
	return fmt.Sprintf(`#!/bin/sh
n=$(grep -c -e '"type":"tool_result"' -e '"turnOrigin":"task_notification"' -e '"isCompactSummary":true' -e '"content":"Stop hook feedback:' -e 'Your previous response had no visible output' "$A10N_MOCK_SESSION_FILE")
sed -n "$((n+1-%d))p" <<'CALLS_EOF' | tr '\001' '\n'
%s
CALLS_EOF
`, skip, strings.Join(lines, "\n"))
}

func copyInput(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
