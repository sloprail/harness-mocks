package replay

import core "github.com/sloprail/harness-mocks/internal/replay"

// Script is the generated scenario for debugging: the main script and each
// sub-agent's.
func Script(runDir string) (string, error) { return core.Script(Adapter{}, runDir) }

// the session id every replay runs under: the recording's own differs in every
// run, and the canonicalisation names it as an id
const sessionID = "00000000-0000-4000-8000-0000000000a1"
