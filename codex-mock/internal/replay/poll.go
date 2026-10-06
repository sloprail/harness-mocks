package replay

import (
	"fmt"
	"regexp"
	"strconv"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

var reSession = regexp.MustCompile(`"session_id":(\d+)`)

// appendSessions adds the session ids a tool output names that are not known yet: the receipts of
// commands a call left running (a poll of one names it again, and adds none).
func appendSessions(have []int, output string) []int {
	for _, m := range reSession.FindAllStringSubmatch(output, -1) {
		n, _ := strconv.Atoi(m[1])
		known := false
		for _, h := range have {
			known = known || h == n
		}
		if !known {
			have = append(have, n)
		}
	}
	return have
}

// unifyPoll is a write_stdin call polling a command left running: the session it names is one of the
// receipts the agent was given, which the mock's script finds again by position (its ids are its own).
func unifyPoll(arg map[string]any, sessions []int) (core.Call, error) {
	id, ok := arg["session_id"].(number)
	if !ok {
		return core.Call{}, fmt.Errorf("a write_stdin whose session_id is not a number the agent was given")
	}
	pos := -1
	for i, s := range sessions {
		if float64(s) == id.f {
			pos = i
		}
	}
	if pos < 0 {
		return core.Call{}, fmt.Errorf("a write_stdin of session %v, which no earlier output named", id.f)
	}
	in := map[string]any{"session": pos}
	for _, k := range []string{"yield_time_ms", "max_output_tokens"} {
		if v, present := arg[k]; present {
			n, isNum := v.(number)
			if !isNum {
				return core.Call{}, fmt.Errorf("a write_stdin whose %s is not a number", k)
			}
			in[k] = int(n.f)
		}
	}
	if _, chars := arg["chars"]; chars {
		return core.Call{}, fmt.Errorf("a write_stdin with chars: the mock only polls a session")
	}
	return core.Call{Tool: core.ToolPoll, Input: in}, nil
}
