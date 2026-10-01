package procexec

import (
	"reflect"
	"testing"
)

func TestEnvReplacesInheritedIdentity(t *testing.T) {
	inherited := []string{"PATH=/bin", "SID=outer", "MODE=x"}
	got := Env(inherited, map[string]string{"SID": "inner", "MODE": "", "NEW": "1"})
	want := []string{"PATH=/bin", "NEW=1", "SID=inner"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Env = %v, want %v", got, want)
	}
}
