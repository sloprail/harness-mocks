package replay

import "sort"

// concurrent is the hook log's canonical lines with each group of concurrent
// hooks put in a fixed order: the hooks of one event run side by side, so the
// order they log in is not the behaviour, while the order of the events is. A
// group is the run of consecutive lines about the same event (the payloads and
// what the hook scripts logged for it); sorted is only within it.
func concurrent(objs []map[string]any, lines []string) []string {
	events := groups(objs)
	out := append([]string(nil), lines...)
	for i := 0; i < len(objs); {
		j := i + 1
		for j < len(objs) && events[j] == events[i] {
			j++
		}
		sort.Strings(out[i:j])
		i = j
	}
	return out
}

// groups is the event each logged line belongs to: its own when it names one of
// Cursor's hook events, and else the event of the line before it, as a tag a hook
// script gives its own log line ("closed") is not an event, and the script ran
// for the event whose lines surround it.
func groups(objs []map[string]any) []string {
	out := make([]string, len(objs))
	cur := ""
	for i, o := range objs {
		if e := eventOf(o); hookEvents[e] {
			cur = e
		}
		out[i] = cur
	}
	return out
}

// hookEvents are Cursor's hook events (https://cursor.com/docs/hooks).
var hookEvents = map[string]bool{
	"sessionStart": true, "sessionEnd": true, "preToolUse": true, "postToolUse": true, "postToolUseFailure": true,
	"subagentStart": true, "subagentStop": true, "beforeShellExecution": true, "afterShellExecution": true,
	"beforeMCPExecution": true, "afterMCPExecution": true, "beforeReadFile": true, "afterFileEdit": true,
	"beforeSubmitPrompt": true, "preCompact": true, "stop": true, "afterAgentResponse": true, "afterAgentThought": true,
	"beforeTabFileRead": true, "afterTabFileEdit": true,
}

func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }

// eventOf is the hook event a logged line is about: a payload's own, or the one
// a hook script logged it for (at the top of the line, or in its hook_result).
func eventOf(o map[string]any) string {
	if e := str(o, "hook_event_name"); e != "" {
		return e
	}
	if e := str(o, "event"); e != "" {
		return e
	}
	r, _ := o["hook_result"].(map[string]any)
	return str(r, "event")
}
