package replay

import (
	"fmt"
	"sort"
)

// sortConcurrent sorts the lines of each run of hook payloads that one firing
// of one event produced: the handlers of an event run at the same time, so the
// order they log in is not the behaviour. The order of the runs is, and is kept.
// Lines are in the order of objs, the objects they were made of. A firing is the
// consecutive payloads with the same event, tool call, agent and stop state; a
// log line with no event of its own (a handler that logs only its own name) is
// part of the firing of the lines around it that have none.
func sortConcurrent(lines []string, objs []map[string]any) {
	start := 0
	for i := 1; i <= len(objs); i++ {
		if i == len(objs) || firing(objs[i]) != firing(objs[start]) {
			sort.Strings(lines[start:i])
			start = i
		}
	}
}

// firing identifies the firing a hook payload belongs to.
func firing(o map[string]any) string {
	return fmt.Sprint(o["hook_event_name"], "|", o["tool_use_id"], "|", o["agent_id"], "|", o["stop_hook_active"])
}
