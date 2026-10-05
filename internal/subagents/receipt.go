package subagents

import "encoding/json"

// A spawn receipt is the JSON a harness answers a sub-agent's dispatch with at
// once: the sub-agent's id (agent_id) and a name the harness picks for it
// (nickname).

// ReceiptAgentID is the sub-agent id a receipt's text names; empty when the
// text is no receipt.
func ReceiptAgentID(text string) string {
	var r struct {
		AgentID string `json:"agent_id"`
	}
	if json.Unmarshal([]byte(text), &r) != nil {
		return ""
	}
	return r.AgentID
}

// ReceiptWithoutNickname is a receipt's text with the nickname, which a harness
// picks at random, as <NICKNAME>; any other text is returned as it is.
func ReceiptWithoutNickname(text string) string {
	var r map[string]any
	if json.Unmarshal([]byte(text), &r) != nil || r["agent_id"] == nil || r["nickname"] == nil {
		return text
	}
	r["nickname"] = "<NICKNAME>"
	b, _ := json.Marshal(r)
	return string(b)
}
