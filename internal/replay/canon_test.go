package replay

import "testing"

// A measurement is compared by whether it is zero, not by its value; other
// numbers are left as they are.
func TestMeasuredNumbersAreComparedBySign(t *testing.T) {
	c := New(Rules{Measured: []string{"duration"}})
	got := c.Lines([]map[string]any{{"duration": 1092.5, "other": 7.0}, {"duration": 0.0, "other": 7.0}})
	want := []string{`{"duration":"\u003cpositive\u003e","other":7}`, `{"duration":"\u003czero\u003e","other":7}`}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %s, want %s", i, got[i], want[i])
		}
	}
}
