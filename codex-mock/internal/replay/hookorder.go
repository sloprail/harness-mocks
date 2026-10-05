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
func concurrentGroups(objs []map[string]any) []int {
	group := make([]int, len(objs))
	prev, n := "", -1
	for i, o := range objs {
		if _, isPayload := o["hook_event_name"]; isPayload {
			k := fmt.Sprint(o["hook_event_name"], "|", o["turn_id"], "|", o["tool_use_id"], "|", o["session_id"], "|", o["agent_id"])
			if k != prev || n < 0 {
				n++
			}
			prev = k
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
