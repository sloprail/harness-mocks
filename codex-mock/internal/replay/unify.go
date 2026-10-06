package replay

import (
	"fmt"
	"math"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// execOptions are the options of an exec_command the mock implements (runner.execParams, by the
// recordings' names): a recording whose call passes another is not replayed.
var execOptions = map[string]bool{"cmd": true, "workdir": true, "yield_time_ms": true, "max_output_tokens": true, "shell": true, "login": true, "tty": true}

// unify is the unified call of one of codex's tool calls.
func unify(m jsCall, spawns []int, told []string) (core.Call, error) {
	var arg map[string]any
	if m.Name == "apply_patch" && len(m.Args) == 1 { // its one argument is the patch text, not an object
		patch, ok := m.Args[0].(string)
		if !ok {
			return core.Call{}, fmt.Errorf("an apply_patch whose argument is not a string")
		}
		return core.Call{Tool: core.ToolPatch, Input: map[string]any{"patch": patch}}, nil
	}
	if len(m.Args) != 1 {
		return core.Call{}, fmt.Errorf("the model called tools.%s with %d arguments: the adapter maps one", m.Name, len(m.Args))
	}
	arg, isObject := m.Args[0].(map[string]any)
	if !isObject {
		return core.Call{}, fmt.Errorf("the model called tools.%s with something other than an object: the adapter maps only an object", m.Name)
	}
	switch m.Name {
	case "exec_command":
		cmd, ok := arg["cmd"].(string)
		if !ok {
			return core.Call{}, fmt.Errorf("an exec_command whose cmd is not a string")
		}
		in := map[string]any{"command": cmd}
		for k, v := range arg { // the options the mock implements go to it as given; one it does not is refused, not ignored
			if !execOptions[k] {
				return core.Call{}, fmt.Errorf("an exec_command with the option %s, which the mock does not implement", k)
			}
			if k == "cmd" || k == "yield_time_ms" {
				continue
			}
			if wd, _ := v.(string); k == "workdir" && wd != "<RUN>" && !strings.HasPrefix(wd, "<RUN>/") { // the run's directory, or one under it a -C named
				return core.Call{}, fmt.Errorf("an exec_command run in %v: the mock runs in the run's directory only", v)
			}
			sv, ok := scalar(v)
			if !ok {
				return core.Call{}, fmt.Errorf("an exec_command whose %s is not a string, number or boolean", k)
			}
			in[k] = sv
		}
		if y, present := arg["yield_time_ms"]; present {
			n, ok := y.(number)
			if !ok {
				return core.Call{}, fmt.Errorf("an exec_command whose yield_time_ms is not a number")
			}
			if n.f != math.Trunc(n.f) || math.Abs(n.f) > 1<<31 {
				return core.Call{}, fmt.Errorf("an exec_command whose yield_time_ms is not a whole number of milliseconds")
			}
			in["yield_time_ms"] = int(n.f)
		}
		return core.Call{Tool: core.ToolShell, Input: in}, nil
	case "multi_agent_v1__spawn_agent":
		if len(arg) == 0 { // a call with no arguments, which the harness refuses (recorded: runs/agent-input-validation)
			return core.Call{Tool: core.ToolSpawn, Input: map[string]any{}}, nil
		}
		msg, ok := arg["message"].(string)
		if !ok {
			return core.Call{}, fmt.Errorf("a spawn_agent whose message is not a string")
		}
		in := map[string]any{"message": msg}
		for k, v := range arg {
			if k == "message" {
				continue
			}
			sv, ok := scalar(v)
			if !ok {
				return core.Call{}, fmt.Errorf("a spawn_agent whose %s is not a string, number or boolean", k)
			}
			in[k] = sv
		}
		return core.Call{Tool: core.ToolSpawn, Input: in}, nil
	case "multi_agent_v1__wait_agent":
		return unifyWait(arg, spawns, told)
	}
	return core.Call{}, fmt.Errorf("the model called tools.%s: the adapter maps exec_command, spawn_agent, wait_agent and apply_patch", m.Name)
}
