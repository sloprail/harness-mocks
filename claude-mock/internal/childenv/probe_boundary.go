package childenv

import "github.com/sloprail/harness-mocks/internal/hooks/probeinner"

// probeBoundary imports a module package that is not its api (rule probe).
const probeBoundary = probeinner.Inner
