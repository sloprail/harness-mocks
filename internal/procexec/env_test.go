package procexec

import (
	"reflect"
	"testing"
)

func TestEnvReplacesInheritedIdentity(t *testing.T) {
	inherited := []string{"PATH=/bin", "SID=outer", "MODE=x"}
	got := Env(inherited, map[string]string{"SID": "inner", "MODE": "", "NEW": "1"}, nil)
	want := []string{"PATH=/bin", "NEW=1", "SID=inner"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Env = %v, want %v", got, want)
	}
}

func TestEnvDefaultsYieldToAnInheritedValue(t *testing.T) {
	dflt := map[string]string{"ENTRY": "own"}
	if got, want := Env([]string{"ENTRY=launcher"}, nil, dflt), []string{"ENTRY=launcher"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("inherited: Env = %v, want %v", got, want)
	}
	// an empty inherited value is no value
	if got, want := Env([]string{"ENTRY="}, nil, dflt), []string{"ENTRY=own"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("empty: Env = %v, want %v", got, want)
	}
	if got, want := Env(nil, nil, dflt), []string{"ENTRY=own"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("absent: Env = %v, want %v", got, want)
	}
}
