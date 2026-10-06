package replay

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// A recorded flag the mock models is passed on, one it refuses says so (and is
// not a listed failure), and one it does not model is an adapter gap.
func TestParseArgs(t *testing.T) {
	got, err := parseArgs("--max-turns\n2\n")
	if err != nil || !reflect.DeepEqual(got, []string{"--max-turns", "2"}) {
		t.Fatalf("modelled: %v %v", got, err)
	}
	_, err = parseArgs("--max-budget-usd\n5\n")
	var u *Unbuildable
	if !errors.As(err, &u) || !strings.HasPrefix(u.Reason, RefusedPrefix) {
		t.Fatalf("refused: %v", err)
	}
	if _, err = parseArgs("--system-prompt\nx\n"); !errors.As(err, &u) || strings.HasPrefix(u.Reason, RefusedPrefix) {
		t.Fatalf("unmodelled: %v", err)
	}
	if _, err = parseArgs("--max-turns\n"); err == nil {
		t.Fatal("a flag short of its value")
	}
	if got, err := parseArgs(""); err != nil || len(got) != 0 {
		t.Fatalf("none: %v %v", got, err)
	}
}
