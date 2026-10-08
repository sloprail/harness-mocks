package replay

import (
	"encoding/json"
	"fmt"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
	"github.com/sloprail/harness-mocks/internal/scenario"
)

// scriptFor is the mock script that plays the model's steps, a call or an
// answer each, in the order it took them. The step to play is the number of
// calls it has been answered (function_call_output records) plus the number of
// times it was sent on after an answer (hook_prompt records, and the end of a sub-agent
// told right after an answer), each of which spent an answer. The steps are inside the script, so a sub-agent's script is a file of
// its own. Call ids are unique across the run's scripts (tag), as the real ones
// are: the mock keys a still-running command by its call's id. Past the last
// step (an answer) the last step is played again. A script of a later run of the harness
// (a resume, a fork) starts after the base steps the session already holds.
func scriptFor(tag string, base int, steps []modelCall, final string, unfinished bool, finalGate scenario.Gate) string {
	steps = append(steps, modelCall{Final: &final, Hang: unfinished, Gate: finalGate})
	var lines []string
	for _, c := range steps {
		if c.Hang { // an agent the recording shows still at work: it does not end
			lines = append(lines, `{"hang":true}`)
			continue
		}
		if c.Final != nil {
			b, _ := json.Marshal(map[string]any{"final": *c.Final, "gate": c.Gate})
			lines = append(lines, string(b))
			continue
		}
		if c.Name == core.ToolCompact {
			lines = append(lines, fmt.Sprintf(`{"type":"compact","trigger":%q}`, c.Input["trigger"]))
			continue
		}
		content := []any{}
		if c.Text != nil {
			content = append(content, map[string]any{"type": "text", "text": *c.Text})
		}
		content = append(content, map[string]any{"type": "tool_use", "id": "", "name": c.Name, "input": c.Input, "more": c.More})
		line := map[string]any{"type": "assistant", "message": map[string]any{"content": content}}
		if !c.Gate.None() {
			line["gate"] = c.Gate
		}
		b, _ := json.Marshal(line)
		lines = append(lines, string(b))
	}
	return fmt.Sprintf(`#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
k=$(( $(grep -c '"type":"compacted"' "$A10N_MOCK_SESSION_FILE") + $(grep -c '<hook_prompt' "$A10N_MOCK_SESSION_FILE") + $(jq -s '[.[]|select(.type=="response_item")|.payload] as $p | [range(1;($p|length)) | select((($p[.].role=="user" and ($p[.].content|tostring|test("subagent_notification"))) or $p[.].role=="developer") and $p[.-1].role=="assistant")] | length' "$A10N_MOCK_SESSION_FILE") ))
steps=$(cat <<'STEPS_EOF'
%s
STEPS_EOF
)
step=$(printf '%%s\n' "$steps" | sed -n "$((n+k+1-%d))p")
[ -n "$step" ] || step=$(printf '%%s\n' "$steps" | tail -1)
case "$step" in
'{"hang":true}') exec sleep 86400 ;;
'{"type":"compact"'*) printf '%%s\n' "$step" ;;
'{"final":'*)
  text=$(printf '%%s' "$step" | jq -c .final)
  gate=$(printf '%%s' "$step" | jq -c '.gate // {}')
  printf '%%s\n' "$(jq -nc --argjson t "$text" --argjson g "$gate" '{gate:$g,type:"assistant",message:{content:[{type:"text",text:$t}]}}')" "$(jq -nc --argjson t "$text" '{type:"result",subtype:"success",result:$t}')"
  ;;
*)
  ids=$(jq -cs '[.[]|select(.payload.type=="function_call_output")|.payload.output|try (fromjson|.agent_id) catch empty|select(.!=null)]' "$A10N_MOCK_SESSION_FILE")
  sess=$(jq -cs '[.[]|select(.payload.type=="function_call_output")|.payload.output|try (fromjson|.session_id) catch empty|select(.!=null)]|reduce .[] as $x ([]; if index($x) != null then . else . + [$x] end)' "$A10N_MOCK_SESSION_FILE")
  printf '%%s\n' "$step" | jq -c --argjson ids "$ids" --argjson sess "$sess" --arg id "call_%s_$n" '.message.content |= map(if .type=="tool_use" then .id = $id | (if (.input.session_id|type) == "object" then .input.session_id |= ($sess[.session] // error("no session receipt at position \(.session)")) else . end) | (if .input.targets then .input.targets |= map(if type=="object" then ($ids[.spawned] // error("no spawn receipt at position \(.spawned)")) else . end) else . end) else . end)'
  ;;
esac
`, strings.Join(lines, "\n"), base, tag)
}
