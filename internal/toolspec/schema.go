// Package toolspec checks the tool calls a scenario script asks a mock to make,
// before the mock plays them. A harness declares the tools its mock implements
// as a Schema, grounded in the recorded runs (each tool and parameter is one a
// recording shows the real harness take); Check refuses, naming the tool and the
// parameter, whatever the schema does not allow.
package toolspec

import (
	"fmt"
	"strings"
)

// Type is the JSON type of a parameter.
type Type string

// The JSON types a parameter can have.
const (
	String  Type = "string"
	Number  Type = "number"
	Integer Type = "integer"
	Boolean Type = "boolean"
	Object  Type = "object"
	Array   Type = "array"
)

// Param is one parameter of a tool.
type Param struct {
	// Name is the parameter's name in a script's call.
	Name string
	// Recorded is its name in the recordings, when they say it another way ("" is the same).
	Recorded string
	Type     Type
	Required bool
	// MockOnly: the mock's own parameter, which the real tool does not have (a
	// sub-agent's script); no recording shows it.
	MockOnly bool
	// Doc is the page of the harness's docs that names the parameter, for one the mock
	// implements and no recorded run happens to show; it grounds the parameter in
	// place of a recording.
	Doc string
	// Unmodeled, when set, says of a value the mock does not implement why (a form
	// of the value no recording shows); "" is a value it does.
	Unmodeled func(v any) string
	// Values are the only values of this option the mock implements; none: any value of its type.
	Values []any
}

// Tool is one tool the mock implements.
type Tool struct {
	// Name is the tool's name in a script's call; one ending in "*" stands for
	// every name with that prefix (the tools of MCP servers, named by the script).
	Name string
	// Recorded is its name in the recordings, when they say it another way ("" is the same).
	Recorded string
	Params   []Param
	// Open: the tool takes whatever arguments its caller names, which the harness
	// does not fix (an MCP server's tool); the input need only be an object.
	Open bool
	// Answers are the kinds of mistake the real harness answers itself, each with the
	// recorded run that shows its answer: the mock answers it as recorded instead of
	// refusing it. A kind with no recording behind it is not listed.
	Answers map[Kind]string
}

// Schema is the tools one harness's mock implements.
type Schema struct {
	Harness string
	Tools   []Tool
}

// Kind is what is wrong with a call.
type Kind string

// The kinds of mistake a call can hold.
const (
	UnknownTool  Kind = "unknown tool"
	UnknownParam Kind = "unknown parameter"
	Missing      Kind = "missing required parameter"
	WrongType    Kind = "wrong type"
	BadValue     Kind = "option value not implemented"
)

// Issue is one mistake in a call.
type Issue struct {
	Kind        Kind
	Tool, Param string
	Detail      string
}

func (i Issue) String() string {
	if i.Param == "" {
		return fmt.Sprintf("%s: %s %s", i.Tool, i.Kind, i.Detail)
	}
	return fmt.Sprintf("%s: %s %q %s", i.Tool, i.Kind, i.Param, i.Detail)
}

// Tool is the schema's tool of that name.
func (s Schema) Tool(name string) (Tool, bool) {
	for _, t := range s.Tools {
		if t.Name == name || strings.HasSuffix(t.Name, "*") && strings.HasPrefix(name, strings.TrimSuffix(t.Name, "*")) {
			return t, true
		}
	}
	return Tool{}, false
}
