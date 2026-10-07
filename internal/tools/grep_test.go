package tools

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGrepSkipsHiddenAndBinaryFiles(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{"a.txt": "alpha\n", ".hidden/b.txt": "alpha\n", ".dot.txt": "alpha\n", "bin.dat": "alpha\x00\n"} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	res, err := Grep(GrepQuery{Pattern: "alpha", Dir: dir})
	require.NoError(t, err)
	require.Len(t, res.Hits, 1)
	assert.Equal(t, "a.txt", res.Hits[0].Name)
}

func TestGrepErrors(t *testing.T) {
	dir := t.TempDir()
	_, err := Grep(GrepQuery{Pattern: "(", Dir: dir})
	var parse *ParseError
	require.True(t, errors.As(err, &parse))
	assert.Contains(t, parse.Diagnostic, "error: unclosed group")
	_, err = Grep(GrepQuery{Pattern: "a", Dir: dir, Path: "nope"})
	assert.ErrorIs(t, err, ErrNoSuchPath)
}

func TestGrepGlobWithASlashMatchesThePath(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a/x.go", "b/x.go"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("hit\n"), 0o644))
	}
	res, err := Grep(GrepQuery{Pattern: "hit", Dir: dir, Glob: "a/*.go"})
	require.NoError(t, err)
	require.Len(t, res.Hits, 1)
	assert.Equal(t, "a/x.go", res.Hits[0].Name)
}
