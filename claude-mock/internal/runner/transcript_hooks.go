package runner

import (
	"bytes"
	"encoding/json"
	"os"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

type heldRun struct {
	in   hooks.Input
	runs []hooks.HandlerRun
}

// holdHookRuns is a recorder that keeps a fire's runs to be written later,
// for records that real Claude Code writes after something the hook decided
// on (flushHookRuns writes them, dropHeldHookRuns forgets them).
func (t *transcript) holdHookRuns(in hooks.Input, runs []hooks.HandlerRun) {
	t.held = append(t.held, heldRun{in: in, runs: runs})
}

func (t *transcript) flushHookRuns() {
	held := t.held
	t.held = nil
	for _, h := range held {
		t.recordHookRuns(h.in, h.runs)
	}
}

func (t *transcript) dropHeldHookRuns() { t.held = nil }

// lastUUIDs is the uuids of the last n records written that carry one, oldest
// first.
func (t *transcript) lastUUIDs(n int) []string {
	if t == nil || n <= 0 {
		return []string{}
	}
	data, err := os.ReadFile(t.path)
	if err != nil {
		return []string{}
	}
	var all []string
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		var rec struct {
			UUID string `json:"uuid"`
		}
		if json.Unmarshal(line, &rec) == nil && rec.UUID != "" {
			all = append(all, rec.UUID)
		}
	}
	if len(all) > n {
		all = all[len(all)-n:]
	}
	if all == nil {
		return []string{}
	}
	return all
}

// recordedEvents are the events whose hooks leave records in a transcript —
// the ones there is evidence for (real transcripts and controlled claude
// 2.1.282 runs; EVIDENCE.md). SessionEnd's output left nothing in a controlled
// run, PreCompact/PostCompact's is display text only (the 2.1.282 binary), and
// there is no evidence at all for WorktreeCreate/WorktreeRemove: no record.
var recordedEvents = map[hooks.EventName]bool{
	hooks.EventSessionStart:     true,
	hooks.EventUserPromptSubmit: true,
	hooks.EventPreToolUse:       true,
	hooks.EventPostToolUse:      true,
	hooks.EventStop:             true,
	hooks.EventSubagentStart:    true,
	hooks.EventSubagentStop:     true,
}

// additionalContext writes the hook_additional_context record that follows a
// hook's hook_success when its JSON carried additionalContext. SessionStart's
// is named after the event alone, with the event as its toolUseID — the shape
// claude 2.1.282 wrote in a controlled run and in 9 real transcripts; every
// other event's shares the hook_success's name and id.
func (t *transcript) additionalContext(in hooks.Input, hookName, toolUseID, ac string) {
	if in.HookEventName == hooks.EventSessionStart {
		hookName, toolUseID = string(hooks.EventSessionStart), string(hooks.EventSessionStart)
	}
	t.persistMap(map[string]any{"type": "attachment", "attachment": map[string]any{
		"type": "hook_additional_context", "content": []string{ac},
		"hookName": hookName, "toolUseID": toolUseID, "hookEvent": string(in.HookEventName),
	}})
}

// isDeny reports a PreToolUse JSON refusal: permissionDecision deny, or the
// deprecated top-level decision:block.
func isDeny(out hooks.Output) bool {
	return out.Decision == "block" ||
		(out.HookSpecificOutput != nil && out.HookSpecificOutput.PermissionDecision == "deny")
}

// denyReason is the text a PreToolUse JSON refusal is quoted with:
// permissionDecisionReason, else reason, else "Blocked by hook" (claude
// 2.1.282's hook-output parser).
func denyReason(out hooks.Output) string {
	if out.HookSpecificOutput != nil && out.HookSpecificOutput.PermissionDecisionReason != "" {
		return out.HookSpecificOutput.PermissionDecisionReason
	}
	if out.Reason != "" {
		return out.Reason
	}
	return "Blocked by hook"
}
