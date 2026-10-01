package session

import (
	"crypto/rand"
	"fmt"
	"io"
)

// NewID is a random identifier in the shape of a version 4 UUID, for a session
// or a record. It returns the error of the random source rather than an
// identifier that is not random, so the caller reports it.
func NewID() (string, error) { return newID(rand.Reader) }

func newID(src io.Reader) (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(src, b); err != nil {
		return "", fmt.Errorf("session id: %w", err)
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
