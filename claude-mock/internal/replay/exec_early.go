package replay

// execEarly are the calls of sub-agents that the stream shows carried out before the next step of the
// other agents: the call's own sub-agent frame is followed by the task_started of the shell it ran
// with no message frame of another agent between them. Another sample of the same run may show the
// other order (the real harness races); each sample's script carries its own, as recorded.
func execEarly(stream []map[string]any) map[string]bool {
	out := map[string]bool{}
	for i, f := range stream {
		if f["subtype"] != "task_started" || f["task_type"] != "local_bash" || f["owned_by_subagent"] != true {
			continue
		}
		id, _ := f["tool_use_id"].(string)
		from, thread := callFrame(stream[:i], id)
		if from < 0 {
			continue
		}
		early := true
		for _, g := range stream[from+1 : i] {
			if messageOfAnotherThread(g, thread) {
				early = false
				break
			}
		}
		out[id] = early
	}
	return out
}

// callFrame is where the stream has the assistant frame that makes the call, and the thread (the
// parent_tool_use_id) of the agent that made it.
func callFrame(frames []map[string]any, id string) (int, any) {
	for i, f := range frames {
		msg, _ := f["message"].(map[string]any)
		blocks, _ := msg["content"].([]any)
		for _, b := range blocks {
			if block, _ := b.(map[string]any); f["type"] == "assistant" && block["type"] == "tool_use" && block["id"] == id {
				return i, f["parent_tool_use_id"]
			}
		}
	}
	return -1, nil
}

// messageOfAnotherThread is whether a frame is a message (not the model's thinking alone) of an agent
// other than the one that made the call.
func messageOfAnotherThread(f map[string]any, thread any) bool {
	if f["type"] != "assistant" && f["type"] != "user" {
		return false
	}
	if f["parent_tool_use_id"] == thread {
		return false
	}
	msg, _ := f["message"].(map[string]any)
	blocks, _ := msg["content"].([]any)
	for _, b := range blocks {
		if block, _ := b.(map[string]any); block["type"] != "thinking" {
			return true
		}
	}
	return false
}

// startAfterPost are the Agent calls (of the main agent or of a sub-agent) whose sub-agent the harness started after the call's PostToolUse
// in this sample: the hooks' payloads (in the order they were logged) show it. Another sample may show the
// other order; each carries its own.
func startAfterPost(payloads []map[string]any, subs map[string]turns) map[string]bool {
	byAgent := map[string]string{} // a sub-agent's id -> the id of the call that started it
	for call, t := range subs {
		byAgent[t.agentID] = call
	}
	posted := map[string]bool{}
	out := map[string]bool{}
	for _, p := range payloads {
		switch p["hook_event_name"] {
		case "PostToolUse":
			if id, _ := p["tool_use_id"].(string); id != "" {
				posted[id] = true
			}
		case "SubagentStart":
			id, _ := p["agent_id"].(string)
			if call, ok := byAgent[id]; ok && posted[call] {
				out[call] = true
			}
		}
	}
	return out
}
