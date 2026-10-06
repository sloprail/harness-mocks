package replay

import "sort"

// sortConcurrent sorts each run of consecutive log lines that are no event's
// payload: the handlers of one event run at the same time, and what each logs of
// its own (its name, its result) comes in no fixed order, while the payloads
// they were all given are the same text, so their order says nothing and the
// firings' order (the payload lines, in place) is the behaviour. lines are in
// the order of objs, the objects they were made of.
func sortConcurrent(lines []string, objs []map[string]any) {
	start := -1
	for i := 0; i <= len(objs); i++ {
		own := i < len(objs) && objs[i]["hook_event_name"] == nil
		switch {
		case own && start < 0:
			start = i
		case !own && start >= 0:
			sort.Strings(lines[start:i])
			start = -1
		}
	}
}
