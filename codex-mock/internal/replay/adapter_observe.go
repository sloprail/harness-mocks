package replay

import core "github.com/sloprail/harness-mocks/internal/replay"

// observe is a recording's event stream and hook payloads with the mock's, each
// side canonicalised under rules, the event stream first: it names the ids in a fixed order.
func observe(rules core.Rules, async map[string]bool, recStream, recHooks, mockStream, mockHooks, recCalls, mockCalls []map[string]any) (want, got core.Observed) {
	wantC, gotC := core.New(rules), core.New(rules)
	want.Events, got.Events = wantC.Lines(recStream), gotC.Lines(mockStream)
	want.Hooks, got.Hooks = wantC.Lines(recHooks), gotC.Lines(mockHooks)
	want.Calls, got.Calls = wantC.Lines(recCalls), gotC.Lines(mockCalls) // the rollouts' tool calls: ids are named after the stream's
	// hooks of one event run at the same time, so the order they log in is not the behaviour:
	// the order of the groups of hooks that run together is (hookorder.go)
	want.Hooks = sortWithinGroups(want.Hooks, concurrentGroups(recHooks, async))
	got.Hooks = sortWithinGroups(got.Hooks, concurrentGroups(mockHooks, async))
	return want, got
}
