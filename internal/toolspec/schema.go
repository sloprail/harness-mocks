// Package toolspec checks the tool calls a scenario script asks a mock to make,
// before the mock plays them. A harness declares the tools its mock implements
// as a Schema, grounded in the recorded runs (each tool and parameter is one a
// recording shows the real harness take); Check refuses, naming the tool and the
// parameter, whatever the schema does not allow.
package toolspec

import "fmt"

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
	// Values are the only values of this option the mock implements; none: any value of its type.
	Values []any
}

// Tool is one tool the mock implements.
type Tool struct {
	// Name is the tool's name in a script's call.
	Name string
	// Recorded is its name in the recordings, when they say it another way ("" is the same).
	Recorded string
	Params   []Param
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
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}
