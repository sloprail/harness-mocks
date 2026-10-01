package subagents

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMissingFromDispatch(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  []string
	}{
		{`{"description":"d","prompt":"p"}`, nil},
		{`{"description":"d"}`, []string{"prompt"}},
		{`{"prompt":"p","subagent_type":"x"}`, []string{"description"}},
		{`{}`, []string{"description", "prompt"}},
		{`not json`, []string{"description", "prompt"}},
	} {
		if got := MissingFromDispatch(json.RawMessage(tc.input)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("MissingFromDispatch(%s) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
