package replay

import (
	"fmt"
	"sort"
)

// concurrentGroups says which hook-log lines belong to one run of hooks that
// run at the same time: the payloads of one event of one turn, one tool call,
// one session and one agent, with the lines the hooks print themselves between
// them. Hooks of a group log in whatever order they finish, so only the order
// within a group is not behaviour: the order of the groups is.
//
// Hooks marked async run in the background and log in whatever order they finish, whichever event
// started them (the real harness logged a start hook and a prompt hook in either order, recorded
// twice): the lines of events whose hooks are async, with nothing else between them, are one group.
func concurrentGroups(objs []map[string]any, async map[string]bool) []int {
	group := make([]int, len(objs))
	prev, n, inAsync := "", -1, false
	for i, o := range objs {
		if ev, isPayload := o["hook_event_name"]; isPayload {
			k := fmt.Sprint(ev, "|", o["turn_id"], "|", o["tool_use_id"], "|", o["session_id"], "|", o["agent_id"], "|", o["stop_hook_active"])
			isAsync := async[fmt.Sprint(ev)]
			if n < 0 || (k != prev && !(isAsync && inAsync)) {
				n++
			}
			prev, inAsync = k, isAsync
		} else if n < 0 {
			n = 0
		}
		group[i] = n
	}
	return group
}

// sortWithinGroups sorts each group's lines, keeping the groups where they are.
func sortWithinGroups(lines []string, group []int) []string {
	out := append([]string(nil), lines...)
	for from := 0; from < len(out); {
		to := from
		for to < len(out) && group[to] == group[from] {
			to++
		}
		sort.Strings(out[from:to])
		from = to
	}
	return out
}
