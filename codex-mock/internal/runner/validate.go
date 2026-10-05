package runner

import "github.com/sloprail/harness-mocks/internal/toolspec"

// refusals is the run's: the calls its agents' scripts asked for that the mock does
// not implement. A mock process is one run, so one set serves the main agent's
// turn and every sub-agent's.
var refusals toolspec.Refusals

// Tools is the schema every call of a script is checked against before the
// host is given it (turnloop.Validator).
func (h turnHost) Tools() toolspec.Schema { return schema }

// Refusals is where a refused call is kept, so the run ends with it whichever
// agent's script asked (turnloop.Validator).
func (h turnHost) Refusals() *toolspec.Refusals { return &refusals }

// Tools is the sub-agent's: the same schema.
func (h subHost) Tools() toolspec.Schema { return schema }

// Refusals is the run's, shared with the main agent's.
func (h subHost) Refusals() *toolspec.Refusals { return &refusals }
