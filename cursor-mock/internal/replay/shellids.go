package replay

import (
	"strconv"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// nameShellIDs puts on each background shell call of the main agent the id the
// harness gave its shell, as the stream's completed frame shows it (shellId): a
// later wait names the shell it means by that id. The i-th shell the agent left
// in the background is the i-th the stream shows started in the background; a run
// whose shells and frames do not pair is not replayed.
func nameShellIDs(a *core.Agent, stream []map[string]any, session string) error {
	var ids []string
	for _, f := range stream {
		if f["type"] != "tool_call" || f["subtype"] != "completed" || f["session_id"] != session {
			continue
		}
		call, _ := f["tool_call"].(map[string]any)
		shell, _ := call["shellToolCall"].(map[string]any)
		result, _ := shell["result"].(map[string]any)
		success, _ := result["success"].(map[string]any)
		if bg, _ := result["isBackground"].(bool); bg {
			if n, ok := success["shellId"].(float64); ok {
				ids = append(ids, strconv.Itoa(int(n)))
			}
		}
	}
	var calls []*core.Call
	for i := range a.Calls {
		if c := &a.Calls[i]; c.Tool == core.ToolShell && c.Input["block_until_ms"] == float64(0) {
			calls = append(calls, c)
		}
	}
	if len(calls) != len(ids) {
		return unbuildable("the agent left %d shells in the background and the stream shows %d: which id is whose is not told", len(calls), len(ids))
	}
	for i, c := range calls {
		in := make(map[string]any, len(c.Input)+1)
		for k, v := range c.Input {
			in[k] = v
		}
		in["task_id"] = ids[i]
		c.Input = in
	}
	return nil
}
