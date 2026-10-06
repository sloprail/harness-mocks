package scenario

import "encoding/json"

type block struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Thinking string          `json:"thinking"`
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	// fields are the rest of a thinking block: what the harness says of the model
	// that thought, which the core does not read.
	fields map[string]json.RawMessage
}

// UnmarshalJSON reads a block, keeping the other keys of a thinking block.
func (b *block) UnmarshalJSON(data []byte) error {
	type plain block
	if err := json.Unmarshal(data, (*plain)(b)); err != nil {
		return err
	}
	if b.Type == "thinking" {
		if err := json.Unmarshal(data, &b.fields); err != nil {
			return err
		}
		delete(b.fields, "type")
		delete(b.fields, "thinking")
	}
	return nil
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
// turn: an assistant line's text blocks are messages and its tool_use blocks
// are the turn's calls (the first also being Tool); a result line is the run's
// end; a compact line asks for a compaction of the session, which ends the turn
// like a call does. Once a call is read, the turn goes on only through further
// assistant lines of tool_use blocks alone, the other calls of the same turn;
// any other line ends it unread.
func (t *Turn) read(raw []byte) (done bool, err error) {
	if t.Tool != nil {
		return !t.addCalls(raw), nil
	}
	var l line
	if err := json.Unmarshal(raw, &l); err != nil {
		return false, err
	}
	switch l.Type {
	case "compact":
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		delete(fields, "type")
		delete(fields, "trigger")
		t.Compact = &Compact{Trigger: l.Trigger, Fields: fields}
		return true, nil
	case "result":
		t.Result = &l.Result
		return true, nil
	case "assistant":
		for _, b := range l.Message.Content {
			switch b.Type {
			case "text":
				if len(t.Tools) == 0 {
					t.Texts = append(t.Texts, b.Text)
				}
			case "thinking":
				if len(t.Tools) == 0 {
					t.Thoughts = append(t.Thoughts, Thought{Text: b.Thinking, Fields: b.fields})
				}
			case "tool_use":
				t.Tools = append(t.Tools, ToolUse{ID: b.ID, Name: b.Name, Input: b.Input})
			}
		}
		if len(t.Tools) > 0 {
			t.Tool = &t.Tools[0]
			return false, nil
		}
	}
	return false, nil
}

// addCalls takes the calls of a line that follows the turn's first and
// reports whether it was one: an assistant line of tool_use blocks alone.
func (t *Turn) addCalls(raw []byte) bool {
	var l line
	if json.Unmarshal(raw, &l) != nil || l.Type != "assistant" || len(l.Message.Content) == 0 {
		return false
	}
	for _, b := range l.Message.Content {
		if b.Type != "tool_use" {
			return false
		}
	}
	for _, b := range l.Message.Content {
		t.Tools = append(t.Tools, ToolUse{ID: b.ID, Name: b.Name, Input: b.Input})
	}
	t.Tool = &t.Tools[0]
	return true
}
