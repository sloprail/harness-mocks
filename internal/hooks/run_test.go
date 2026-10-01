package hooks

import (
	"context"
	"testing"
	"time"
)

var rt = Runtime{Env: []string{"PATH=/usr/bin:/bin", "X=1"}, DefaultTimeout: 5 * time.Second}

func TestRunAllGivesEachCommandThePayloadAndKeepsTheOrder(t *testing.T) {
	rt := rt
	rt.Dir = t.TempDir()
	out := RunAll(context.Background(), []Command{
		{Line: `sleep 0.3; cat; echo "slow $X"; exit 2`},
		{Line: `cat; echo err >&2`},
	}, []byte("payload"), rt)
	if len(out) != 2 {
		t.Fatalf("outcomes = %+v", out)
	}
	if out[0].Exit != 2 || out[0].Stdout != "payloadslow 1\n" || !out[0].Started {
		t.Errorf("first = %+v", out[0])
	}
	if out[1].Exit != 0 || out[1].Stdout != "payload" || out[1].Stderr != "err\n" {
		t.Errorf("second = %+v", out[1])
	}
}

func TestRunAllStartsCommandsTogether(t *testing.T) {
	start := time.Now()
	RunAll(context.Background(), []Command{{Line: "sleep 0.5"}, {Line: "sleep 0.5"}, {Line: "sleep 0.5"}}, nil, rt)
	if took := time.Since(start); took > 1200*time.Millisecond {
		t.Fatalf("three half-second commands took %v: they ran one after another", took)
	}
}

func TestRunAllTimeoutEndsOnlyThatCommand(t *testing.T) {
	out := RunAll(context.Background(), []Command{
		{Line: "sleep 30", Timeout: 200 * time.Millisecond}, {Line: "echo fine"},
	}, nil, rt)
	if !out[0].TimedOut || out[0].Exit != -1 {
		t.Errorf("slow = %+v, want a timeout", out[0])
	}
	if out[1].TimedOut || out[1].Stdout != "fine\n" {
		t.Errorf("fast = %+v", out[1])
	}
}

func TestMatches(t *testing.T) {
	for _, tc := range []struct {
		matcher, value string
		aliases        []string
		want           bool
	}{
		{"", "Bash", nil, true}, {"*", "Bash", nil, true},
		{"Bash", "Bash", nil, true}, {"^Bash$", "Bash", nil, true}, {"^Bash$", "BashX", nil, false},
		{"Edit|Write", "apply_patch", []string{"Edit", "Write"}, true}, {"Edit", "apply_patch", nil, false},
		{"mcp__fs__.*", "mcp__fs__read", nil, true}, {"[", "Bash", nil, false},
	} {
		if got := Matches(tc.matcher, tc.value, tc.aliases...); got != tc.want {
			t.Errorf("Matches(%q, %q, %v) = %v, want %v", tc.matcher, tc.value, tc.aliases, got, tc.want)
		}
	}
}

func TestPreToolRefusalOneRefusalStandsAndTheFirstReasonIsTold(t *testing.T) {
	allow := PreToolVote{}
	deny := PreToolVote{Denied: true, DenyReason: "denied"}
	block := PreToolVote{Blocked: true, BlockReason: "blocked"}
	for _, tc := range []struct {
		votes      []PreToolVote
		wantRefuse bool
		wantReason string
	}{
		{nil, false, ""}, {[]PreToolVote{allow, allow}, false, ""},
		{[]PreToolVote{allow, deny}, true, "denied"}, {[]PreToolVote{deny, allow}, true, "denied"},
		{[]PreToolVote{block, deny}, true, "blocked"}, {[]PreToolVote{deny, block}, true, "denied"},
	} {
		if r, why := PreToolRefusal(tc.votes); r != tc.wantRefuse || why != tc.wantReason {
			t.Errorf("PreToolRefusal(%+v) = (%v, %q), want (%v, %q)", tc.votes, r, why, tc.wantRefuse, tc.wantReason)
		}
	}
}
