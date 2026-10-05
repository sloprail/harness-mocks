package replay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// Unbuildable is the core's: what of a recording the adapter cannot reproduce.
type Unbuildable = core.Unbuildable

func unbuildable(err error) error { return &core.Unbuildable{Reason: err.Error()} }

var (
	reTool  = regexp.MustCompile(`tools\.(\w+)\(`)
	reExec  = regexp.MustCompile(`(?s)tools\.exec_command\(\{(.*?)\}\)`)
	reSpawn = regexp.MustCompile(`(?s)tools\.multi_agent_v1__spawn_agent\(\{message:\s*("(?:[^"\\]|\\.)*")`)
	reYield = regexp.MustCompile(`yield_time_ms:\s*(\d+)`)
	reCmd   = regexp.MustCompile(`cmd:\s*("(?:[^"\\]|\\.)*")`)
)

// modelTurns are the calls the model made in one recorded rollout, in order,
// and its final answer. Codex's model calls its tools from a JS `exec` call
// (tools.exec_command({...}), tools.multi_agent_v1__spawn_agent({...})), so
// the calls are read out of that JS (codex's own tool names are mapped to the
// unified ones here); a call that only looks around (ALL_TOOLS)
// makes none. What the adapter cannot map is an error, never a guess.
func modelTurns(rollout string) (agent core.Agent, err error) {
	var calls []core.Call
	var final string
	var said *string
	for _, rec := range jsonLines(rollout) {
		p, _ := rec["payload"].(map[string]any)
		if rec["type"] != "response_item" || p == nil {
			continue
		}
		switch {
		case p["type"] == "function_call" || p["type"] == "custom_tool_call" && p["name"] != "exec":
			return core.Agent{}, fmt.Errorf("the model called %v: the adapter maps only exec", p["name"])
		case p["type"] == "message" && p["role"] == "assistant":
			text := ""
			for _, c := range p["content"].([]any) {
				text += c.(map[string]any)["text"].(string)
			}
			if p["phase"] == "final_answer" {
				final = text
			} else {
				said = &text
			}
		case p["type"] == "custom_tool_call":
			js, _ := p["input"].(string)
			for _, m := range reTool.FindAllStringSubmatch(js, -1) {
				switch m[1] {
				case "exec_command", "multi_agent_v1__spawn_agent", "multi_agent_v1__wait_agent":
				default:
					return core.Agent{}, fmt.Errorf("the model called tools.%s: the adapter maps exec_command and spawn_agent", m[1])
				}
			}
			for _, m := range reExec.FindAllStringSubmatch(js, -1) {
				var cmd string
				c := reCmd.FindStringSubmatch(m[1])
				if c == nil || json.Unmarshal([]byte(c[1]), &cmd) != nil {
					return core.Agent{}, fmt.Errorf("an exec_command whose cmd is not a string literal")
				}
				in := map[string]any{"command": cmd}
				if y := reYield.FindStringSubmatch(m[1]); y != nil {
					n, _ := strconv.Atoi(y[1])
					in["yield_time_ms"] = n
				}
				calls = append(calls, core.Call{Said: said, Tool: core.ToolShell, Input: in})
				said = nil
			}
			for _, m := range reSpawn.FindAllStringSubmatch(js, -1) {
				var msg string
				if json.Unmarshal([]byte(m[1]), &msg) != nil {
					return core.Agent{}, fmt.Errorf("a spawn_agent whose message is not a string literal")
				}
				calls = append(calls, core.Call{Said: said, Tool: core.ToolSpawn, Input: map[string]any{"message": msg}})
				said = nil
			}
		}
	}
	return core.Agent{Calls: calls, Final: final}, nil
}

// the command line every replayable recording was made with: the mock is run
// with the equivalent flags, and a recording made another way (a resume, a -c
// override, an output schema) is not replayed by this adapter.
const standardCommand = "codex exec --json --skip-git-repo-check --dangerously-bypass-approvals-and-sandbox --dangerously-bypass-hook-trust -m gpt-5.6-luna"

// sampleDir is the latest sample of the run in dir; empty when it has none.
func sampleDir(dir string) string {
	samples, _ := filepath.Glob(filepath.Join(dir, "samples", "*"))
	if len(samples) == 0 {
		return ""
	}
	return samples[len(samples)-1]
}

// Load reads the recorded run in runDir (run.yaml, setup/, samples/) into the
// unified form: the main agent's calls with the sub-agents it spawned attached,
// in the order they were spawned. An *Unbuildable says what the adapter cannot
// reproduce.
func (Adapter) Load(runDir string) (core.Recording, error) {
	if fi, err := os.Stat(runDir); err != nil || !fi.IsDir() {
		return core.Recording{}, fmt.Errorf("%s is not a recorded run", runDir)
	}
	setup, sample := filepath.Join(runDir, "setup"), sampleDir(runDir)
	cmd := ""
	for _, l := range strings.Split(readFile(filepath.Join(runDir, "run.yaml")), "\n") {
		if v, ok := strings.CutPrefix(l, "command: "); ok {
			cmd = v
		}
	}
	if cmd != standardCommand {
		return core.Recording{}, unbuildable(fmt.Errorf("recorded with another command line: %q", cmd))
	}
	entries, _ := os.ReadDir(setup)
	for _, e := range entries {
		if n := e.Name(); n != "hooks.json" && n != "hook.sh" && n != "prompt.txt" {
			return core.Recording{}, unbuildable(fmt.Errorf("the setup has %s, which the adapter does not install", n))
		}
	}
	if sample == "" {
		return core.Recording{}, unbuildable(fmt.Errorf("no sample was recorded"))
	}
	paths, _ := filepath.Glob(filepath.Join(sample, "transcript", "*.jsonl"))
	if len(paths) == 0 {
		return core.Recording{}, unbuildable(fmt.Errorf("no rollout was recorded: the model's turns are unknown"))
	}
	main := ""
	for _, l := range jsonLines(readFile(filepath.Join(sample, "stream.jsonl"))) {
		if l["type"] == "thread.started" {
			main, _ = l["thread_id"].(string)
		}
	}
	var mainRollout string
	var subs []string
	for _, p := range paths {
		if strings.Contains(p, main) {
			mainRollout = readFile(p)
		} else {
			subs = append(subs, readFile(p))
		}
	}
	if mainRollout == "" {
		return core.Recording{}, unbuildable(fmt.Errorf("no rollout of the main thread"))
	}
	agent, err := modelTurns(mainRollout)
	if err != nil {
		return core.Recording{}, unbuildable(err)
	}
	n := 0
	for i := range agent.Calls {
		if agent.Calls[i].Tool != core.ToolSpawn {
			continue
		}
		if n >= len(subs) {
			return core.Recording{}, unbuildable(fmt.Errorf("a spawn_agent call with no recorded sub-agent rollout"))
		}
		sub, err := modelTurns(subs[n])
		if err != nil {
			return core.Recording{}, unbuildable(fmt.Errorf("sub-agent: %w", err))
		}
		agent.Calls[i].Sub = &sub
		n++
	}
	return core.Recording{
		Dir:    runDir,
		Prompt: strings.TrimSpace(readFile(filepath.Join(setup, "prompt.txt"))),
		Setup: map[string]string{
			"hooks.json": readFile(filepath.Join(setup, "hooks.json")),
			"hook.sh":    readFile(filepath.Join(setup, "hook.sh")),
		},
		Agent: agent,
	}, nil
}
