package replay

import core "github.com/sloprail/harness-mocks/internal/replay"

// compactions follows the compactions of one transcript: a boundary says how the compaction that
// the next summary record belongs to was made.
type compactions struct {
	written   map[string]bool // the uuids the file holds
	trigger   string
	preserved int    // how many of the last messages it kept
	logical   string // "unwritten" when its logical parent was a record the file does not hold
	segment   bool   // whether it names a preserved segment
}

func newCompactions(records []map[string]any) *compactions {
	c := &compactions{written: map[string]bool{}, trigger: "manual", segment: true}
	for _, r := range records {
		if id, _ := r["uuid"].(string); id != "" {
			c.written[id] = true
		}
	}
	return c
}

// see takes a record: a boundary sets what the next summary is of, and a summary is the compaction
// as a call, the user record the harness writes with isCompactSummary, whose text is what the agent
// is given in place of what was compacted.
func (c *compactions) see(rec map[string]any) (core.Call, bool) {
	if meta, _ := rec["compactMetadata"].(map[string]any); rec["subtype"] == "compact_boundary" && meta != nil {
		c.trigger, _ = meta["trigger"].(string)
		c.preserved, c.logical = 0, ""
		_, c.segment = meta["preservedSegment"]
		if lp, _ := rec["logicalParentUuid"].(string); lp != "" && !c.written[lp] {
			c.logical = "unwritten"
		}
		if pm, _ := meta["preservedMessages"].(map[string]any); pm != nil {
			uuids, _ := pm["uuids"].([]any)
			c.preserved = len(uuids)
		}
		return core.Call{}, false
	}
	if rec["type"] != "user" || rec["isCompactSummary"] != true {
		return core.Call{}, false
	}
	msg, _ := rec["message"].(map[string]any)
	text, ok := msg["content"].(string)
	in := map[string]any{"summary": text, "trigger": c.trigger}
	if c.preserved > 0 {
		in["preserve"] = c.preserved
	}
	if c.logical != "" {
		in["logical_parent"] = c.logical
	}
	if !c.segment {
		in["preserved_segment"] = false
	}
	return core.Call{Tool: core.ToolCompact, Input: in}, ok
}

// attachSummarizerOutput gives each manual compaction of the agent what the summarizer wrote: the
// last_assistant_message of the SubagentStop the compaction fired (agent_type "", the summarizer), in
// order. The summary record keeps it in the form the agent reads; the hooks are told this.
func attachSummarizerOutput(a *core.Agent, payloads []map[string]any) {
	var outputs []string
	for _, p := range payloads {
		if p["hook_event_name"] == "SubagentStop" && p["agent_type"] == "" {
			text, _ := p["last_assistant_message"].(string)
			outputs = append(outputs, text)
		}
	}
	for i := range a.Calls {
		if c := &a.Calls[i]; c.Tool == core.ToolCompact && c.Input["trigger"] == "manual" && len(outputs) > 0 {
			c.Input["model_output"] = outputs[0]
			outputs = outputs[1:]
		}
	}
}
