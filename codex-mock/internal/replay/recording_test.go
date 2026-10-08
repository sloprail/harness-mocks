package replay

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A line of a recording that is not JSON is an error that names it, not a line
// the replay quietly leaves out of what it compares.
func TestReadJSONLRefusesAnUnparsableLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payloads.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("{\"a\":1}\n\nnot json\n{\"b\":2}\n"), 0o644))
	_, err := readJSONL(path)
	var u *Unbuildable
	require.True(t, errors.As(err, &u))
	assert.Contains(t, err.Error(), "line 3 is not a JSON object")
}

func TestReadJSONLKeepsEveryObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("{\"a\":1}\n\n{\"b\":2}\n"), 0o644))
	got, err := readJSONL(path)
	require.NoError(t, err)
	assert.Len(t, got, 2)
}
