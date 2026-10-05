package runner

import (
	"context"
	"encoding/json"

	"github.com/sloprail/harness-mocks/internal/scenario"
)

// refusesTaskModel answers a Task call whose model parameter names a model the
// agent may not use: the call's preToolUse hooks fire, and it completes with an
// error naming the model, with no started frame and no after-tool hook
// (recorded: runs/foreground-subagent-failure, a Task call with the model
// no-such-model-xyz). The models recorded for a sub-agent are "default" and
// "inherit" (runs/subagent-worktree-isolation); any other is answered so. The recorded error goes on to list the models the
// account may choose from, which is that account's catalogue and is not
// modeled: the mock lists "default". It reports whether it answered.
//
// sr:provides foreground-subagent-result/cursor
func (s *session) refusesTaskModel(ctx context.Context, tu scenario.ToolUse) bool {
	var in taskInput
	if tu.Name != "Task" || json.Unmarshal(tu.Input, &in) != nil || in.Model == nil || *in.Model == "default" || *in.Model == "inherit" {
		return false
	}
	_, args := s.announceTask(ctx, tu, in)
	args["model"] = *in.Model
	s.tr.toolUse(tu.Name, map[string]any{"description": in.Description, "prompt": in.Prompt})
	msg := "Invalid model selection \"" + *in.Model + "\". Model could not be resolved to a valid subagent model.\nAllowed model slugs:\n- default"
	s.forward(taskFrame(s.id, tu.ID, "completed", args, map[string]any{"error": map[string]any{"error": msg}}))
	return true
}
