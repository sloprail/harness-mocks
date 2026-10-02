package toolcall

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/sloprail/harness-mocks/internal/hooks"
)

// fake is a host that records what the call did, in order.
type fake struct {
	steps   []string
	refuse  bool
	failed  bool
	block   bool
	answers []Answer
}

func (f *fake) Tool(name string) ([]string, bool) { return []string{"command"}, name == "Bash" }
func (f *fake) Before(context.Context, Call) (bool, string) {
	f.steps = append(f.steps, "before")
	return f.refuse, "no"
}
func (f *fake) Execute(context.Context, Call) Result {
	f.steps = append(f.steps, "execute")
	return Result{Output: "out", Failed: f.failed}
}
func (f *fake) After(_ context.Context, _ Call, _ Result, kind hooks.AfterTool) (string, bool) {
	f.steps = append(f.steps, map[hooks.AfterTool]string{hooks.AfterSuccess: "after-success", hooks.AfterFailure: "after-failure"}[kind])
	return "feedback", f.block
}
func (f *fake) Answer(_ Call, a Answer) { f.answers = append(f.answers, a) }

func call(name, input string) Call { return Call{ID: "c1", Name: name, Input: json.RawMessage(input)} }

func TestRunOrder(t *testing.T) {
	for _, tc := range []struct {
		name  string
		f     fake
		c     Call
		o     Options
		steps []string
		kind  Kind
	}{
		{"unknown tool: no hook", fake{}, call("Nope", `{}`), Options{}, nil, Unknown},
		{"missing input: no hook", fake{}, call("Bash", `{}`), Options{}, nil, Invalid},
		{"refused: no run, no after hook", fake{refuse: true}, call("Bash", `{"command":"x"}`), Options{}, []string{"before"}, Refused},
		{"refused, reported as a failure", fake{refuse: true}, call("Bash", `{"command":"x"}`), Options{SeparateFailureHook: true, FailureOnRefusal: true},
			[]string{"before", "after-failure"}, Refused},
		{"refused, failure option without a failure hook", fake{refuse: true}, call("Bash", `{"command":"x"}`), Options{FailureOnRefusal: true},
			[]string{"before"}, Refused},
		{"success", fake{}, call("Bash", `{"command":"x"}`), Options{}, []string{"before", "execute", "after-success"}, Done},
		{"failure, own hook", fake{failed: true}, call("Bash", `{"command":"x"}`), Options{SeparateFailureHook: true},
			[]string{"before", "execute", "after-failure"}, Done},
		{"failure, one hook for both", fake{failed: true}, call("Bash", `{"command":"x"}`), Options{},
			[]string{"before", "execute", "after-success"}, Done},
	} {
		Run(context.Background(), &tc.f, tc.c, tc.o)
		if !reflect.DeepEqual(tc.f.steps, tc.steps) {
			t.Errorf("%s: steps = %v, want %v", tc.name, tc.f.steps, tc.steps)
		}
		if len(tc.f.answers) != 1 || tc.f.answers[0].Kind != tc.kind {
			t.Errorf("%s: answers = %+v, want one of kind %d", tc.name, tc.f.answers, tc.kind)
		}
	}
}

func TestRunAnswerCarriesWhatHappened(t *testing.T) {
	f := fake{refuse: true}
	Run(context.Background(), &f, call("Bash", `{"command":"x"}`), Options{})
	if a := f.answers[0]; a.Reason != "no" {
		t.Errorf("refused answer = %+v", a)
	}
	f = fake{}
	Run(context.Background(), &f, call("Bash", `{}`), Options{})
	if a := f.answers[0]; !reflect.DeepEqual(a.Missing, []string{"command"}) {
		t.Errorf("invalid answer = %+v", a)
	}
	f = fake{block: true, failed: true}
	Run(context.Background(), &f, call("Bash", `{"command":"x"}`), Options{})
	if a := f.answers[0]; !a.Replaced || a.Feedback != "feedback" || a.Result.Output != "out" || !a.Result.Failed {
		t.Errorf("blocked answer = %+v", a)
	}
}
