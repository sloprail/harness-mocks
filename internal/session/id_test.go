package session

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

var v4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewIDIsAV4UUIDAndNotRepeated(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		id := NewID()
		require.Regexp(t, v4, id)
		require.False(t, seen[id], "NewID repeated %s", id)
		seen[id] = true
	}
}
