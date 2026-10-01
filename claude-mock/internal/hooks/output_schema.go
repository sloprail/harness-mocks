package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// schemaInvalid is the message a hook's JSON output that parses but fails
// Claude Code's output schema leaves: the failing field, then what it allows
// (claude 2.1.285 on {"decision": 42}: `decision: Invalid option: expected
// one of "approve"|"block"`, recorded: snapshots/runs/hook-exit-json).
const schemaInvalid = "Hook JSON output validation failed — "

// decodeOutput reads a hook's JSON output into out, or says which field fails
// the schema and leaves out empty: output that fails it is not used.
func decodeOutput(stdout []byte, out *Output) string {
	msg := schemaProblem(stdout, out)
	if msg != "" {
		*out = Output{}
	}
	return msg
}

func schemaProblem(stdout []byte, out *Output) string {
	if err := json.Unmarshal(stdout, out); err != nil {
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) && te.Field != "" {
			return schemaInvalid + fieldProblem(te.Field, te.Value)
		}
		return schemaInvalid + err.Error()
	}
	if out.Decision != "" && out.Decision != "approve" && out.Decision != "block" {
		return schemaInvalid + fieldProblem("decision", "string")
	}
	return ""
}

// fieldProblem is the schema's complaint about one field holding a value of
// JSON type got.
func fieldProblem(field, got string) string {
	if field == "decision" {
		return `decision: Invalid option: expected one of "approve"|"block"`
	}
	return fmt.Sprintf("%s: Invalid input: received %s", field, strings.TrimSpace(got))
}
