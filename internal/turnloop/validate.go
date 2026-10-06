package turnloop

import (
	"github.com/sloprail/harness-mocks/internal/scenario"
	"github.com/sloprail/harness-mocks/internal/toolspec"
)

// Validator is a Host whose script's tool calls are checked, before the host is
// given them (adr/tool-calls-validated): against the schema of the tools its
// mock implements. A call the schema refuses ends the turn with the refusal, and
// is kept in the run's Refusals, which every agent's session in the run shares,
// so the run ends with it whichever agent's script asked.
type Validator interface {
	Tools() toolspec.Schema
	Refusals() *toolspec.Refusals
}

// validate checks the calls of a turn of the script; a mistake the harness
// answers itself passes on to the host, which answers it.
func validate(h Host, t scenario.Turn) error {
	v, ok := h.(Validator)
	if !ok {
		return nil
	}
	for _, c := range t.Tools {
		if _, err := v.Tools().Check(c.Name, c.Input); err != nil {
			v.Refusals().Set(err)
			return err
		}
	}
	return nil
}

// refusedBy is the refusal an agent of the run has already earned, if any.
func refusedBy(h Host) error {
	if v, ok := h.(Validator); ok {
		return v.Refusals().Err()
	}
	return nil
}
