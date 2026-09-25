package e2e

import (
	"testing"

	"github.com/sloprail/harness-mocks/claude-mock/e2etest"
)

func TestMain(m *testing.M) { e2etest.Main(m) }

var (
	run             = e2etest.Run
	runWithScript   = e2etest.RunWithScript
	runInDirWithEnv = e2etest.RunInDir
	writeScript     = e2etest.WriteScript
)
