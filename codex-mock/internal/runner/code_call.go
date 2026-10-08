package runner

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"github.com/sloprail/harness-mocks/internal/scenario"
	"github.com/sloprail/harness-mocks/internal/toolspec"
)

// codeOf is the JS Codex's model writes for a tool call, as every recorded rollout shows it: the
// call is made from its code-mode `exec` tool, in the recorded name of the tool and of its
// parameters (a script says Bash and command; the recordings say exec_command and cmd):
//
//	const r = await tools.exec_command({cmd:"ls",workdir:"<dir>"}); text(r.output);
//	const r = await tools.apply_patch("*** Begin Patch ..."); text(r);
//	const r = await tools.multi_agent_v1__spawn_agent({message:"..."}); text(JSON.stringify(r));
//
// The mock's own parameters (a spawn's script) are not the harness's and are left out. A tool the
// schema does not know is called under its own name, with its input as it is.
func codeOf(tu scenario.ToolUse) string {
	var in map[string]json.RawMessage
	_ = json.Unmarshal(tu.Input, &in)
	tool, known := schema.Tool(tu.Name)
	if !known {
		return "const r = await tools." + tu.Name + "(" + object(in, names(in)) + "); text(JSON.stringify(r));\n"
	}
	name := tool.RecordedName()
	switch {
	case tu.Name == patchTool: // the patch is the one argument, as it is
		return "const r = await tools." + name + "(" + literal(in["command"]) + "); text(r);\n"
	case tu.Name == toolName || tu.Name == stdinTool:
		return "const r = await tools." + name + "(" + params(tool, in) + "); text(r.output);\n"
	}
	return "const r = await tools." + name + "(" + params(tool, in) + "); text(JSON.stringify(r));\n"
}

// params is the call's object: the tool's parameters in their order, in their recorded names.
func params(tool toolspec.Tool, in map[string]json.RawMessage) string {
	var parts []string
	for _, p := range tool.Params {
		if raw, given := in[p.Name]; given && !p.MockOnly {
			parts = append(parts, p.RecordedName()+":"+literal(raw))
		}
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// object is the input as an object of the given keys.
func object(in map[string]json.RawMessage, keys []string) string {
	parts := make([]string, len(keys))
	for i, k := range keys {
		key, _ := json.Marshal(k)
		parts[i] = string(key) + ":" + literal(in[k])
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func names(in map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// literal is a JSON value as the JS literal of the same value, with no HTML escaping (JSON is JS).
func literal(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "undefined"
	}
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // a number is written as it was
	if dec.Decode(&v) != nil {
		return string(raw)
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimSuffix(b.String(), "\n")
}
