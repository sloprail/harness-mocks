package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT001_08_VersionReportsBuildVersion verifies that --version prints the
// embedded build version ("dev" for an unstamped build) and exits 0, without a
// script or a prompt.
func TestT001_08_VersionReportsBuildVersion(t *testing.T) {
	out, code := run(t, "--version")
	require.Equal(t, 0, code, "output:\n%s", out)
	assert.Equal(t, "dev\n", out)
}
