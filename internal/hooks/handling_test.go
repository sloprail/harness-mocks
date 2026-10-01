package hooks

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestSelectExactOrRegexp(t *testing.T) {
	for _, tc := range []struct {
		matcher, subject string
		aliases          []string
		want             bool
	}{
		{"", "Bash", nil, true}, {"*", "Bash", nil, true},
		{"Bash", "Bash", nil, true}, {"Bash", "BashX", nil, false}, {"Rea", "Read", nil, false}, {"read", "Read", nil, false},
		{"Edit|Write", "Write", nil, true}, {"Edit|Write", "NotebookEdit", nil, false},
		{"Edit, Write", "Write", nil, true}, {" Edit ,Write", "Edit", nil, true},
		{"code-reviewer", "code-reviewer", nil, true}, {"mcp__memory", "mcp__memory__read", nil, false},
		{"^Rea.*", "Read", nil, true}, {"Edit.*", "NotebookEdit", nil, true}, {"^Edit$", "NotebookEdit", nil, false},
		{"Bash|Read", "apply_patch", []string{"Read"}, true}, {"[", "Bash", nil, false},
	} {
		if got := Select(MatchExactOrRegexp, tc.matcher, tc.subject, tc.aliases...); got != tc.want {
			t.Errorf("Select(exact-or-regexp, %q, %q, %v) = %v, want %v", tc.matcher, tc.subject, tc.aliases, got, tc.want)
		}
	}
	// the regexp style searches every matcher in the subject
	if !Select(MatchRegexp, "Rea", "Read") || !Matches("Edit", "NotebookEdit") {
		t.Error("the regexp style must search the matcher in the subject")
	}
}

func TestActedBlockIsTheLastToFinish(t *testing.T) {
	slowFirst := RunAll(context.Background(), []Command{
		{Line: `sleep 0.4; echo A >&2; exit 2`}, {Line: `echo B >&2; exit 2`}, {Line: "exit 0"},
	}, nil, rt)
	if i, ok := ActedBlock(slowFirst, false); !ok || i != 0 {
		t.Errorf("slow first: ActedBlock = (%d, %v), want (0, true): the block that finished last", i, ok)
	}
	slowSecond := RunAll(context.Background(), []Command{
		{Line: `echo A >&2; exit 2`}, {Line: `sleep 0.4; echo B >&2; exit 2`}, {Line: "exit 0"},
	}, nil, rt)
	if i, ok := ActedBlock(slowSecond, false); !ok || i != 1 {
		t.Errorf("slow second: ActedBlock = (%d, %v), want (1, true)", i, ok)
	}
	for i, o := range slowSecond {
		if o.Done < 1 || o.Done > 3 {
			t.Errorf("command %d: Done = %d", i, o.Done)
		}
	}
}

func TestActedBlockNoneBlocked(t *testing.T) {
	out := RunAll(context.Background(), []Command{{Line: "exit 0"}, {Line: "exit 1"}, {Line: "exit 3"}}, nil, rt)
	if _, ok := ActedBlock(out, false); ok {
		t.Error("only exit 2 blocks")
	}
	if i, ok := ActedBlock(out, true); !ok || i == 0 {
		t.Errorf("strict: any non-zero exit blocks; got (%d, %v)", i, ok)
	}
}

func TestATimedOutCommandRendersNoDecision(t *testing.T) {
	out := RunAll(context.Background(), []Command{
		{Line: `sleep 5; exit 2`, Timeout: 200 * time.Millisecond}, {Line: "exit 0"},
	}, nil, rt)
	if out[0].Counts() || !out[1].Counts() {
		t.Errorf("counts = %v, %v", out[0].Counts(), out[1].Counts())
	}
	if _, ok := ActedBlock(out, false); ok {
		t.Error("a command that timed out blocked")
	}
	if out[0].Timeout != 200*time.Millisecond || out[0].Took < 200*time.Millisecond || out[0].Took > 4*time.Second {
		t.Errorf("timed-out command: Timeout=%v Took=%v", out[0].Timeout, out[0].Took)
	}
	if got := DefaultTimeout(0, 9*time.Second); got != 9*time.Second {
		t.Errorf("default = %v", got)
	}
	if got := DefaultTimeout(2*time.Second, 9*time.Second); got != 2*time.Second {
		t.Errorf("own = %v", got)
	}
}

func TestATimeoutKillsWhatTheCommandSpawned(t *testing.T) {
	// the grandchild would write after 1s; its group is killed at 0.2s
	marker := t.TempDir() + "/survived"
	RunAll(context.Background(), []Command{
		{Line: `(sleep 1; : > ` + marker + `) & sleep 30`, Timeout: 200 * time.Millisecond},
	}, nil, rt)
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Error("a process the timed-out command spawned outlived it")
	}
}

func TestRecordFor(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    Ran
		want Record
	}{
		{"silent success", Ran{Moment: MomentPreTool}, Record{}},
		{"printed output", Ran{Moment: MomentPostTool, Printed: true}, Record{Attachment: AttachSuccess}},
		{"output with context", Ran{Moment: MomentPostTool, Printed: true, Context: true}, Record{Attachment: AttachSuccess, Context: true}},
		{"a prompt hook's context stands alone", Ran{Moment: MomentPrompt, Printed: true, Context: true}, Record{Context: true}},
		{"a prompt hook's plain output", Ran{Moment: MomentPrompt, Printed: true}, Record{Attachment: AttachSuccess}},
		{"failed status", Ran{Moment: MomentStop, FailedStatus: true}, Record{Attachment: AttachNonBlockingError}},
		{"json error", Ran{Moment: MomentPreTool, JSONError: true}, Record{Attachment: AttachNonBlockingError}},
		{"cancelled", Ran{Moment: MomentPreTool, Cancelled: true}, Record{Attachment: AttachCancelled}},
		{"start blocked is no block", Ran{Moment: MomentSessionStart, Blocked: true}, Record{Attachment: AttachNonBlockingError}},
		{"subagent start blocked", Ran{Moment: MomentSubagentStart, Blocked: true}, Record{Attachment: AttachNonBlockingError}},
		{"stop blocked by status", Ran{Moment: MomentStop, Blocked: true}, Record{Feedback: true}},
		{"stop blocked by decision", Ran{Moment: MomentSubagentStop, DecisionBlock: true}, Record{Attachment: AttachBlockingError, Feedback: true}},
		{"after a tool, a block", Ran{Moment: MomentPostTool, Blocked: true}, Record{Attachment: AttachBlockingError}},
		{"after a failed tool, a block", Ran{Moment: MomentToolFailed, Blocked: true}, Record{Attachment: AttachBlockingError}},
		{"after a tool, a decision block", Ran{Moment: MomentPostTool, DecisionBlock: true}, Record{Attachment: AttachBlockingError}},
		{"a refused call leaves none", Ran{Moment: MomentPreTool, Blocked: true}, Record{}},
		{"a deny leaves none", Ran{Moment: MomentPreTool, Deny: true, Printed: true}, Record{}},
		{"session end leaves none", Ran{Moment: MomentSessionEnd, Printed: true, FailedStatus: true}, Record{}},
		{"compaction leaves none", Ran{Moment: MomentCompaction, Printed: true}, Record{}},
		{"worktree leaves none", Ran{Moment: MomentWorktree, Blocked: true}, Record{}},
	} {
		if got := RecordFor(tc.r); got != tc.want {
			t.Errorf("%s: RecordFor(%+v) = %+v, want %+v", tc.name, tc.r, got, tc.want)
		}
	}
	if !SummaryAfter(MomentStop, 1) || SummaryAfter(MomentStop, 0) || SummaryAfter(MomentSubagentStop, 1) || SummaryAfter(MomentPreTool, 2) {
		t.Error("only a Stop that ran a hook ends with a summary")
	}
}

func TestHTTPResult(t *testing.T) {
	for _, tc := range []struct {
		name              string
		reached, timedOut bool
		status            int
		body              string
		want              HTTPOutcome
	}{
		{"empty 2xx", true, false, 200, "", HTTPAccepted},
		{"blank 204", true, false, 204, " \n", HTTPAccepted},
		{"json object", true, false, 200, ` {"decision":"block"} `, HTTPDecided},
		{"plain text 2xx", true, false, 200, "ok", HTTPNonBlockingError},
		{"json array", true, false, 200, "[1]", HTTPNonBlockingError},
		{"500", true, false, 500, `{"decision":"block"}`, HTTPNonBlockingError},
		{"404", true, false, 404, "", HTTPNonBlockingError},
		{"redirect", true, false, 302, "", HTTPNonBlockingError},
		{"unreachable", false, false, 0, "", HTTPNonBlockingError},
		{"timeout", false, true, 0, "", HTTPCancelled},
		{"timeout after a reply", true, true, 200, "{}", HTTPCancelled},
	} {
		if got := HTTPResult(tc.reached, tc.timedOut, tc.status, tc.body); got != tc.want {
			t.Errorf("%s: HTTPResult = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestPluginContributes(t *testing.T) {
	for _, tc := range []struct{ enabled, declared, want bool }{
		{true, true, true}, {false, true, false}, {true, false, false}, {false, false, false},
	} {
		if got := PluginContributes(tc.enabled, tc.declared); got != tc.want {
			t.Errorf("PluginContributes(%v, %v) = %v, want %v", tc.enabled, tc.declared, got, tc.want)
		}
	}
}
