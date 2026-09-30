package runner

import (
	"encoding/json"
	"strings"
)

// stopSummary accumulates one Stop fire's stop_hook_summary.
type stopSummary struct {
	toolUseID  string
	infos      []map[string]any
	errors     []string
	contexts   []string
	hasOutput  bool
	prevented  bool
	stopReason string
}

// writeStopSummary writes the system record real Claude Code writes after
// every Stop fire that ran at least one hook (8,272 in the real transcripts,
// all in main files; none for SubagentStop). A handler that blocked is listed
// without durationMs, as claude 2.1.282 lists it.
func (t *transcript) writeStopSummary(s stopSummary) {
	if len(s.infos) == 0 {
		return
	}
	errs := s.errors
	if errs == nil {
		errs = []string{}
	}
	ctxs := s.contexts
	if ctxs == nil {
		ctxs = []string{}
	}
	t.persistMap(map[string]any{
		"type": "system", "subtype": "stop_hook_summary",
		"hookCount": len(s.infos), "hookInfos": s.infos, "hookErrors": errs,
		"hookAdditionalContext": ctxs, "preventedContinuation": s.prevented,
		"stopReason": s.stopReason, "hasOutput": s.hasOutput, "level": "suggestion",
		"toolUseID": s.toolUseID,
	})
}

func looksLikeJSONObject(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") {
		return false
	}
	var v map[string]any
	return json.Unmarshal([]byte(s), &v) == nil
}

// stopHookFeedback writes the record real Claude Code puts ahead of a blocking
// Stop's attachment: a meta user turn carrying the reason, which is what the
// re-prompted agent reads. In the real transcripts the
// "Stop hook feedback:\n<reason>" user record sits immediately before the
// hook_blocking_error attachment (1,417 Stop, 472 SubagentStop), in the main
// file for Stop and in the sub-agent's own file for SubagentStop.
func (t *transcript) stopHookFeedback(reason string) {
	if t == nil {
		return
	}
	t.persistMap(map[string]any{
		"type":    "user",
		"isMeta":  true,
		"message": map[string]any{"role": "user", "content": "Stop hook feedback:\n" + reason},
	})
}
