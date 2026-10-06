package toolexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sloprail/harness-mocks/internal/tools"
)

// editInput is the argument shape for the Edit tool.
// sr:docs https://code.claude.com/docs/en/tools-reference#edit-tool-behavior
type editInput struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

// CheckInput answers a call whose input is well-formed but cannot be acted on
// before any hook sees it: an Edit whose string is absent or ambiguous
// (recorded: snapshots/runs/file-tools: no hook fired for either). ok is false
// for every other call, and for an Edit that can go ahead (or whose file cannot
// be read: that call fails when it runs).
func CheckInput(toolName string, raw json.RawMessage, cwd string) (Result, bool) {
	var inp editInput
	if toolName != "Edit" || json.Unmarshal(raw, &inp) != nil || inp.FilePath == "" {
		return Result{}, false
	}
	content, err := tools.ReadFile(resolvePath(inp.FilePath, cwd))
	if err != nil {
		return Result{}, false
	}
	ref, refused := tools.RefusedEdit(content, inp.OldString, inp.ReplaceAll)
	if !refused {
		return Result{}, false
	}
	msg := "String to replace not found in file.\nString: " + inp.OldString
	if !ref.Absent {
		msg = fmt.Sprintf("Found %d matches of the string to replace, but replace_all is false. To replace all occurrences, set replace_all to true. To replace only one occurrence, please provide more context to uniquely identify the instance.\nString: %s", ref.Matches, inp.OldString)
	}
	return Result{Output: "<tool_use_error>" + msg + "</tool_use_error>", IsError: true, ToolUseResult: "Error: " + msg}, true
}

// executeEdit replaces an exact string in a file (recorded:
// snapshots/runs/file-tools).
//
// sr:provides file-tools/claude
func executeEdit(ctx context.Context, raw json.RawMessage, cwd, sessionID string) Result {
	var inp editInput
	if err := json.Unmarshal(raw, &inp); err != nil || inp.FilePath == "" {
		return Result{Output: "Edit: missing or invalid 'file_path' field", IsError: true}
	}
	path := resolvePath(inp.FilePath, cwd)
	content, err := tools.ReadFile(path)
	if errors.Is(err, tools.ErrNotFound) {
		return failed("File does not exist. Note: your current working directory is " + cwd + ".")
	}
	if err != nil {
		return failed(err.Error())
	}
	updated, _, err := tools.Edit(content, inp.OldString, inp.NewString, inp.ReplaceAll)
	if err != nil { // CheckInput refuses these before the call runs
		return failed(strings.TrimPrefix(err.Error(), "the "))
	}
	if _, _, werr := tools.WriteFile(path, updated); werr != nil {
		return failed(werr.Error())
	}
	// When the agent's context holds the file as it was, the result says it is current and the structured
	// result does not flag the content as outside the model's context (recorded: runs/fgsub-tool-stats,
	// file-tools).
	res := Result{Output: "The file " + path + " has been updated successfully.", ToolUseResult: map[string]any{
		"filePath": path, "oldString": inp.OldString, "newString": inp.NewString, "originalFile": content,
		"structuredPatch": patchOf(content, updated), "userModified": false, "replaceAll": inp.ReplaceAll,
	}}
	if isKnown(ctx, sessionID, path) {
		res.Output += fileStateNote
	} else {
		res.ToolUseResult.(map[string]any)["contentNotInModelContext"] = true
	}
	return res
}
