package hooks

import "github.com/sloprail/harness-mocks/claude-mock/e2etest"

// probeLayer makes the core import a mock package (rule probe).
var probeLayer = &e2etest.MockBinaryPath
