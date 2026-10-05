package runner

import "github.com/sloprail/harness-mocks/internal/toolspec"

// refusals is the run's: the calls its agents' scripts asked for that the mock does
// not implement. A mock process is one run, so one set serves the main agent's
// session and every sub-agent's.
var refusals toolspec.Refusals

// Tools is the schema every call of a script is checked against before the
// session is given it (turnloop.Validator).
func (s *session) Tools() toolspec.Schema { return schema }

// Refusals is where a refused call is kept, so the run ends with it whichever
// agent's script asked (turnloop.Validator).
func (s *session) Refusals() *toolspec.Refusals { return &refusals }
