package session

import (
	"bytes"
	"errors"
	"regexp"
	"testing"
)

var v4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewIDIsAVersion4UUID(t *testing.T) {
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if !v4.MatchString(id) {
		t.Fatalf("not a v4 UUID: %q", id)
	}
}

func TestNewIDsDiffer(t *testing.T) {
	a, _ := NewID()
	b, _ := NewID()
	if a == b {
		t.Fatalf("two ids are both %q", a)
	}
}

func TestNewIDFromKnownBytes(t *testing.T) {
	id, err := newID(bytes.NewReader(bytes.Repeat([]byte{0xff}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	if id != "ffffffff-ffff-4fff-bfff-ffffffffffff" {
		t.Fatalf("got %q", id)
	}
}

type failing struct{}

func (failing) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

func TestNewIDReportsARandomSourceFailure(t *testing.T) {
	id, err := newID(failing{})
	if err == nil || id != "" {
		t.Fatalf("got %q, %v; want no id and an error", id, err)
	}
}
