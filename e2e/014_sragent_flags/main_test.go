package e2e

import (
	"testing"

	"github.com/a10n-build/a10n-cli/services/claude-mock/e2etest"
)

func TestMain(m *testing.M) { e2etest.Main(m) }

var (
	runInDir      = e2etest.RunInDir
	runWithScript = e2etest.RunWithScript
)
