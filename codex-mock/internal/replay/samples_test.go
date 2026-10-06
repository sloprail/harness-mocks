package replay

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every sample of a run is one the mock's output is compared with; the latest is the one
// the model's turns are read from.
func TestSampleDirsAreAllOfThemOldestFirst(t *testing.T) {
	dir := t.TempDir()
	for _, s := range []string{"20261003-2", "20261003-1"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "samples", s), 0o755))
	}
	got := sampleDirs(dir)
	require.Len(t, got, 2)
	assert.Equal(t, "20261003-1", filepath.Base(got[0]))
	assert.Equal(t, "20261003-2", filepath.Base(sampleDir(dir)))
	assert.Empty(t, sampleDir(t.TempDir()))
}
