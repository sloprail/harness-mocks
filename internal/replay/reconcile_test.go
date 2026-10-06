package replay

import (
	"reflect"
	"testing"
)

// Where the recorded samples disagree, a mock line equal to another sample's is
// accepted; a line the samples agree on is never relaxed, and a line no sample
// shows stays a difference.
func TestReconcileAcceptsOnlyARecordedOutcomeWhereSamplesDisagree(t *testing.T) {
	wants := []Observed{
		{Events: []string{"same", "acted A", "end"}},
		{Events: []string{"same", "acted B", "end"}},
	}
	gots := []Observed{
		{Events: []string{"same", "acted B", "end"}}, // sample 0 recorded A: the mock's B is another sample's outcome
		{Events: []string{"same", "acted C", "different"}},
	}
	out := Reconcile(wants, gots)
	if want := []string{"same", "acted A", "end"}; !reflect.DeepEqual(out[0].Events, want) {
		t.Fatalf("an outcome another sample shows: got %v, want %v", out[0].Events, want)
	}
	if want := []string{"same", "acted C", "different"}; !reflect.DeepEqual(out[1].Events, want) {
		t.Fatalf("an outcome no sample shows, and a line the samples agree on: got %v", out[1].Events)
	}
}
