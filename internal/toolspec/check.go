package toolspec

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Error is a call the mock refuses: a scenario script asked for what the mock
// does not implement.
type Error struct {
	Harness string
	Issues  []Issue
}

func (e *Error) Error() string {
	lines := make([]string, len(e.Issues))
	for i, is := range e.Issues {
		lines[i] = is.String()
	}
	return fmt.Sprintf("%s-mock: the scenario script's tool call is refused: %s", e.Harness, strings.Join(lines, "; "))
}

// Check validates a script's call of the named tool with the given input. It
// returns the mistakes the real harness answers itself (the tool's Answers), for
// the mock to answer as recorded, and an *Error when there are others: an
// unknown tool, an unknown parameter, a missing required one, a value of the
// wrong type, or an option value the mock does not implement. The mock fails
// fast on those, never plays the call.
func (s Schema) Check(name string, input json.RawMessage) (answered []Issue, err error) {
	tool, ok := s.Tool(name)
	if !ok {
		return nil, &Error{s.Harness, []Issue{{Kind: UnknownTool, Tool: name, Detail: "(the mock implements: " + s.names() + ")"}}}
	}
	var refused []Issue
	for _, is := range tool.issues(input) {
		if _, answers := tool.Answers[is.Kind]; answers {
			answered = append(answered, is)
		} else {
			refused = append(refused, is)
		}
	}
	if len(refused) > 0 {
		return nil, &Error{s.Harness, refused}
	}
	return answered, nil
}

func (s Schema) names() string {
	names := make([]string, len(s.Tools))
	for i, t := range s.Tools {
		names[i] = t.Name
	}
	return strings.Join(names, ", ")
}

// issues are the mistakes in the call's input, in the order of the tool's
// parameters, the unknown ones last.
func (t Tool) issues(input json.RawMessage) []Issue {
	var in map[string]json.RawMessage
	if err := json.Unmarshal(input, &in); err != nil || in == nil {
		return []Issue{{Kind: WrongType, Tool: t.Name, Detail: "the input is not a JSON object"}}
	}
	if t.Open {
		return nil
	}
	var out []Issue
	for _, p := range t.Params {
		raw, given := in[p.Name]
		delete(in, p.Name)
		if !given {
			if p.Required {
				out = append(out, Issue{Kind: Missing, Tool: t.Name, Param: p.Name})
			}
			continue
		}
		var v any
		_ = json.Unmarshal(raw, &v)
		if !p.Type.holds(v) {
			out = append(out, Issue{Kind: WrongType, Tool: t.Name, Param: p.Name, Detail: fmt.Sprintf("must be a %s, got %s", p.Type, kindOf(v))})
		} else if len(p.Values) > 0 && !p.allows(v) {
			out = append(out, Issue{Kind: BadValue, Tool: t.Name, Param: p.Name, Detail: fmt.Sprintf("is %s; the mock implements %v", raw, p.Values)})
		}
	}
	unknown := make([]string, 0, len(in))
	for name := range in {
		unknown = append(unknown, name)
	}
	sort.Strings(unknown)
	for _, name := range unknown {
		out = append(out, Issue{Kind: UnknownParam, Tool: t.Name, Param: name, Detail: "(the tool takes: " + t.paramNames() + ")"})
	}
	return out
}

func (t Tool) paramNames() string {
	names := make([]string, len(t.Params))
	for i, p := range t.Params {
		names[i] = p.Name
	}
	return strings.Join(names, ", ")
}

func (p Param) allows(v any) bool {
	for _, a := range p.Values {
		if n, isInt := a.(int); isInt { // a number decodes as float64
			a = float64(n)
		}
		if reflect.DeepEqual(a, v) {
			return true
		}
	}
	return false
}
