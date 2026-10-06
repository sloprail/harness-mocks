package toolexec

import (
	"context"
	"encoding/json"

	"github.com/sloprail/harness-mocks/internal/tools"
)

// fileStateNote is what Claude Code appends to the result of a Write
// (recorded: snapshots/runs/file-tools).
const fileStateNote = " (file state is current in your context — no need to Read it back)"

// writeInput is the argument shape for the Write tool.
// sr:docs https://code.claude.com/docs/en/tools-reference#write-tool-behavior
type writeInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

// executeWrite creates or overwrites a file and says which, with the diff of
// an overwrite (recorded: snapshots/runs/file-tools).
//
// sr:provides file-tools/claude
func executeWrite(ctx context.Context, raw json.RawMessage, cwd, sessionID string) Result {
	var inp writeInput
	if err := json.Unmarshal(raw, &inp); err != nil || inp.FilePath == "" {
		return Result{Output: "Write: missing or invalid 'file_path' field", IsError: true}
	}
	path := resolvePath(inp.FilePath, cwd)
	old, existed, err := tools.WriteFile(path, inp.Content)
	if err != nil {
		return failed(err.Error())
	}
	structured := map[string]any{"type": "create", "filePath": path, "content": inp.Content,
		"structuredPatch": []any{}, "originalFile": nil, "userModified": false}
	text := "File created successfully at: " + path
	if existed {
		structured["type"], structured["originalFile"], structured["structuredPatch"] = "update", old, patchOf(old, inp.Content)
		text = "The file " + path + " has been updated successfully."
	}
	setKnown(ctx, sessionID, path, true)
	return Result{Output: text + fileStateNote, ToolUseResult: structured}
}

// patchOf is the structured patch of a change, in the field names Claude Code
// records (oldStart, oldLines, newStart, newLines, lines).
func patchOf(oldText, newText string) []any {
	out := []any{}
	for _, h := range tools.Patch(oldText, newText) {
		out = append(out, map[string]any{
			"oldStart": h.OldStart, "oldLines": h.OldLines, "newStart": h.NewStart, "newLines": h.NewLines, "lines": h.Lines,
		})
	}
	return out
}
