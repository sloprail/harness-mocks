package e2e

import (
	"testing"

	"github.com/sloprail/harness-mocks/claude-mock/e2etest"
)

func TestMain(m *testing.M) { e2etest.Main(m) }

var runInDir = e2etest.RunInDir
