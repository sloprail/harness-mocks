package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFirstDir(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	gone := filepath.Join(a, "deleted")
	file := filepath.Join(a, "file")
	assert.NoError(t, os.WriteFile(file, nil, 0o644))

	assert.Equal(t, a, FirstDir(a, b), "the first that exists")
	assert.Equal(t, b, FirstDir(gone, b), "a missing directory falls back")
	assert.Equal(t, b, FirstDir("", gone, file, b), "empty, missing and non-directories are skipped")
	assert.Equal(t, gone, FirstDir(gone, filepath.Join(a, "also-gone")), "none exists: the first, to fail as it would")
	assert.Equal(t, "", FirstDir())
}
