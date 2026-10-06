package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// ownedBashFrames streams the frames real Claude Code streams for a
// foreground Bash run by a BACKGROUND sub-agent: task_started {owned_by_subagent,
// is_backgrounded:false, task_type:"local_bash"} before it runs and
// task_notification {status, output_file:"", summary:<description>} after
// (F:bgagent). It returns the function that writes the second; the main agent's
// Bash has frames only when it runs long (slowBashFrames), any other call none. A failed command's frame reads "failed": that status is
// not measured.
func ownedBashFrames(cfg Config, call pendingToolUse) func(toolexec.Result) {
	if call.ToolName != "Bash" {
		return func(toolexec.Result) {}
	}
	if !cfg.SuppressSubagentHooks || cfg.SyncSubagent { // the main agent's: a task only once it has run long
		return slowBashFrames(cfg, call)
	}
	id, desc := "b"+randomID(8), bashDescription(call)
	writeTaskStarted(cfg, taskStart{ID: id, ToolUseID: call.ToolUseID, Description: desc, TaskType: "local_bash", OwnedBySubagent: true})
	return func(res toolexec.Result) {
		status := "completed"
		if res.IsError {
			status = "failed"
		}
		writeTaskNotification(cfg, taskNote{ID: id, ToolUseID: call.ToolUseID, Status: status, Summary: desc})
	}
}

// toolResponse is PostToolUse's tool_response for a result.
func toolResponse(res toolexec.Result) json.RawMessage {
	var v any = res.Output
	if res.ToolUseResult != nil {
		v = res.ToolUseResult
	}
	b, _ := marshalRecord(v)
	return b
}

// emitToolResult writes a synthetic user record with a tool_result block to the
// STDOUT stream and, chained into the transcript, to the session FILE.
//
// The record carries no uuid/parentUuid of its own — sw.persist mints a uuid and
// chains it from the previous persisted record, which is exactly the shape a real
// tool_result (type:"user") has on disk: uuid + a non-null parentUuid. The STDOUT
// copy stays uuid-less, matching the mock's claude stream (the stream frames carry no
// transcript uuid; the FILE is where the chained identity lives).
//
// The content is a string — what real Claude Code writes for a tool's text
// result, a refusal and an error alike — or, where the real tool answers with
// one (an async Agent receipt), a list of text blocks.
//
// sr:docs https://docs.anthropic.com/en/docs/claude-code/sdk#stream-json-output-format
func emitToolResult(cfg Config, call pendingToolUse, res toolexec.Result, tr *transcript) error {
	// A result with no text is given to the model as "(<Tool> completed with
	// no output)" — claude 2.1.282 replaces any empty or whitespace-only tool
	// result content with it (3,479 "(Bash completed with no output)" results
	// in the real transcripts, 0 empty ones).
	// sr:provides empty-tool-result-placeholder/claude
	res.Output = toolcall.ResultText(call.ToolName, res.Output)
	var content any = res.Output
	if res.ContentAsBlocks {
		content = []map[string]any{{"type": "text", "text": res.Output}}
	}

	record := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role": "user",
			"content": []map[string]any{
				{
					"type":        "tool_result",
					"tool_use_id": call.ToolUseID,
					"content":     content,
					"is_error":    res.IsError,
				},
			},
		},
	}

	// The stream frame carries the tool's structured result as tool_use_result
	// (the file's record calls it toolUseResult, below).
	frame := map[string]any{}
	for k, v := range record {
		frame[k] = v
	}
	if res.ToolUseResult != nil && cfg.AgentID == "" { // a sub-agent's result frames carry none (recorded: runs/isolated-worktree)
		frame["tool_use_result"] = res.ToolUseResult
	}
	if res.NonExecution != "" {
		frame["tool_result_meta"] = []any{map[string]any{"id": call.ToolUseID, "non_execution_kind": res.NonExecution}}
	}
	if !res.IsError && call.ToolName != "Bash" {
		frame["message"] = withoutIsError(record["message"].(map[string]any)) // only a Bash result says it is not an error (recorded: 83 results)
	}
	line, err := marshalRecord(frame)
	if err != nil {
		return fmt.Errorf("claude-mock: marshal tool_result: %w", err)
	}
	writeStreamLine(cfg, line)

	// The FILE copy carries what real Claude Code puts beside a tool_result:
	// toolUseResult, the tool's structured result (a background launch's
	// backgroundTaskId, an async agent's agentId), where the tool gives one.
	if res.ToolUseResult != nil {
		record["toolUseResult"] = res.ToolUseResult
		if withResult, err := marshalRecord(record); err == nil {
			line = withResult
		}
	}
	tr.persist(line)
	return nil
}

// toolNameInTranscript is the name of the tool_use with id toolUseID in the
// transcript, or "".
func toolNameInTranscript(tr *transcript, toolUseID string) string {
	data, err := os.ReadFile(tr.path)
	if err != nil {
		return ""
	}
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if !bytes.Contains(line, []byte(toolUseID)) {
			continue
		}
		if id, name, _ := extractFirstToolUseWithID(line); id == toolUseID {
			return name
		}
	}
	return ""
}
