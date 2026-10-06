package runner

import "github.com/sloprail/harness-mocks/internal/toolspec"

// Tools is the schema every call of a script is checked against before the agent is given
// it (turnloop.Validator): the main agent's and every sub-agent's.
func (s *state) Tools() toolspec.Schema { return schema }

// Refusals is where a refused call is kept, shared by the main agent's session and every
// sub-agent's, so the run ends with it whichever agent's script asked (turnloop.Validator).
func (s *state) Refusals() *toolspec.Refusals { return s.refused }
