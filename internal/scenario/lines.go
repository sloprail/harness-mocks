package scenario

import "encoding/json"

type block struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type line struct {
	Type    string `json:"type"`
	Trigger string `json:"trigger"`
	Result  string `json:"result"`
	Message struct {
		Content []block `json:"content"`
	} `json:"message"`
}

// read takes one script line into the turn and reports whether it ends the
// turn: an assistant line's text blocks are messages and its first tool_use
// block is the call; a result line is the run's end; a compact line asks for a
// compaction of the session, which ends the turn like a call does.
func (t *Turn) read(raw []byte) (done bool, err error) {
	var l line
	if err := json.Unmarshal(raw, &l); err != nil {
		return false, err
	}
	switch l.Type {
	case "compact":
		t.Compact = &Compact{Trigger: l.Trigger}
		return true, nil
	case "result":
		t.Result = &l.Result
		return true, nil
	case "assistant":
		for _, b := range l.Message.Content {
			switch b.Type {
			case "text":
				t.Texts = append(t.Texts, b.Text)
			case "tool_use":
				t.Tool = &ToolUse{ID: b.ID, Name: b.Name, Input: b.Input}
				return true, nil
			}
		}
	}
	return false, nil
}
