package runner

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/scenario"
)

// emptySearches are the catalogue searches a recording shows finding nothing.
var emptySearches = map[string]bool{
	"subscribe_timer|cursor-subscriptions": true, // runs/schedule-wakeup-ask
	"Task|task|subagent|sub-agent":         true, // runs/catalogue-search-task, the search a sub-agent at the depth limit made (runs/nested-subagents-depth)
	"skill|greet|launch":                   true, // runs/skill-tool-ask, the search for a skill-launching tool
}

// startsHookless starts the calls the harness answers itself, which no hook
// sees: a search of its tool catalogue (GetDynamicTools) and a wait on a
// background task (AwaitShell). It returns the second half of the call, and
// whether the call was one of these.
//
// The catalogue is Cursor's own list of tools, which the mock has none of: a
// search is answered only where a recording shows it finding nothing
// (emptySearches), and any other is refused as not modeled, since the real
// harness answers some with tools (runs/stop-hook-payload, a search that
// matches). A wait with no task named lasts as long as it
// was told to (recorded: runs/nested-subagents-background, a wait of 60000 ms
// that ran 60000 ms with a sub-agent still running); a wait on a named task is
// not modeled, as the task's id is the harness's.
func (s *session) startsHookless(ctx context.Context, tu scenario.ToolUse) (func(), bool) {
	var in map[string]any
	_ = json.Unmarshal(tu.Input, &in)
	switch tu.Name {
	case "GetDynamicTools":
		pattern, _ := in["pattern"].(string)
		c := toolexec.Call{Kind: "getMcpToolsToolCall", Args: map[string]any{"pattern": pattern}}
		s.forward(startedFrame(s.id, tu.ID, c))
		s.tr.toolUse(tu.Name, in)
		return func() {
			s.named = true // the transcript is named once a call, hooks or not, is past (recorded: runs/nested-subagents-depth)
			if !emptySearches[pattern] {
				s.forward(errorFrame(s.id, tu.ID, c, s.refuseMsg("cursor-mock: a search of the tool catalogue for "+strconv.Quote(pattern)+" is not modeled: only a search a recording shows finding nothing is answered"), nil))
				return
			}
			body, _ := json.MarshalIndent(struct {
				Mode    string `json:"mode"`
				Pattern string `json:"pattern"`
				Matches []any  `json:"matches"`
			}{"search", pattern, []any{}}, "", "  ")
			s.forward(completedFrame(s.id, tu.ID, c, map[string]any{"success": map[string]any{"content": string(body)}}, nil))
		}, true
	case "AwaitShell":
		block, _ := in["block_until_ms"].(float64)
		id, named := in["shell_id"].(string)
		c := toolexec.Call{Kind: "awaitToolCall", Args: map[string]any{"taskId": id, "blockUntilMs": int64(block)}}
		s.forward(startedFrame(s.id, tu.ID, c))
		s.tr.toolUse(tu.Name, in)
		return func() {
			s.named = true
			if named {
				s.awaitTask(ctx, tu.ID, c, id, time.Duration(block)*time.Millisecond)
				return
			}
			select {
			case <-time.After(time.Duration(block) * time.Millisecond):
			case <-ctx.Done():
			}
			s.forward(completedFrame(s.id, tu.ID, c, map[string]any{"success": map[string]any{"complete": map[string]any{
				"taskId": "", "runtimeMs": strconv.FormatInt(int64(block), 10), "outputFilePath": "", "outputLength": "0", "regexRequested": false}}}, nil))
		}, true
	}
	return nil, false
}
