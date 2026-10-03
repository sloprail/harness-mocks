package runner

import (
	"github.com/sloprail/harness-mocks/codex-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// patchTool is the file tool: how hooks name it, and the agent's call.
const patchTool = "apply_patch"

// fileOrHookName is the tool's name in the hooks' payloads: apply_patch for
// the file tool, else as hookName has it.
func fileOrHookName(c toolcall.Call) string {
	if c.Name == patchTool {
		return patchTool
	}
	return hookName(c)
}

// failedPatch: a patch that fails fires PreToolUse only (recorded:
// runs/file-tools-failure), so no PostToolUse follows its failure.
func failedPatch(c toolcall.Call) bool { return c.Name == patchTool }

// patchTold is what the agent is told of a patch that was applied (recorded:
// runs/file-tools).
const patchTold = "{}"

// applyPatch writes the files of a patch, and shows them in the event stream; a
// patch that cannot apply writes nothing, and the agent is told why.
func (h toolHost) applyPatch(c toolcall.Call) toolcall.Result {
	changes, response, err := toolexec.Apply(command(c), h.cfg.Cwd)
	if err != nil {
		return toolcall.Result{Output: err.Error(), Failed: true}
	}
	var shown []map[string]string
	for _, ch := range changes {
		shown = append(shown, map[string]string{"path": ch.Path, "kind": ch.Kind})
	}
	h.events.FileChange(shown, "completed")
	return toolcall.Result{Output: response}
}
