package hooks

import "testing"

func TestPreToolRefusalGivenASeparatorTellsEveryRefusalInOrder(t *testing.T) {
	allow := PreToolVote{}
	deny := PreToolVote{Denied: true, DenyReason: "denied"}
	block := PreToolVote{Blocked: true, BlockReason: "blocked"}
	for _, tc := range []struct {
		votes      []PreToolVote
		wantRefuse bool
		wantReason string
	}{
		{nil, false, ""}, {[]PreToolVote{allow, allow}, false, ""},
		{[]PreToolVote{allow, deny}, true, "denied"},
		{[]PreToolVote{deny, allow, block}, true, "denied|blocked"},
		{[]PreToolVote{block, deny, deny}, true, "blocked|denied|denied"},
	} {
		if r, why := PreToolRefusal(tc.votes, "|"); r != tc.wantRefuse || why != tc.wantReason {
			t.Errorf("PreToolRefusal(%+v, |) = (%v, %q), want (%v, %q)", tc.votes, r, why, tc.wantRefuse, tc.wantReason)
		}
	}
}
