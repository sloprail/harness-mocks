package replay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Scenario is what the mock is given to replay a recording: the run's own
// setup (its hooks.json, its hook script, its prompt) and the scenario script
// that makes the model's calls, in the format the mock takes of any scenario.
type Scenario struct {
	HooksJSON string
	// Files are written into the repository (the hook script).
	Files map[string]string
	// Scripts are the sub-agents' scenario scripts. They are not the repository's
	// (the real run's repository holds none), so they sit beside it, at
	// scriptsDir, which the run's own directory replaces.
	Scripts map[string]string
	Script  string
	Prompt  string
}

// scriptsDir stands, in the scripts' calls, for the directory the sub-agents'
// scripts are written to.
const scriptsDir = "@SCRIPTS@"

// Unbuildable says what of a recording the adapter cannot reproduce yet.
type Unbuildable struct{ Reason string }

func (u *Unbuildable) Error() string { return u.Reason }

func unbuildable(err error) error { return &Unbuildable{Reason: err.Error()} }

// modelCall is one tool call the model made, in the mock's script vocabulary.
type modelCall struct {
	Text  *string        `json:"text,omitempty"` // what the model said just before the call, if it said anything
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
	// Final is the end of a turn: the model's answer, with no call. A turn that
	// a Stop hook blocks is followed by the model's next steps, so a run can hold
	// several.
	Final *string `json:"-"`
}

var (
	reTool        = regexp.MustCompile(`tools\.(\w+)\(`)
	reExec        = regexp.MustCompile(`(?s)tools\.exec_command\(\{(.*?)\}\)`)
	reSpawn       = regexp.MustCompile(`(?s)tools\.multi_agent_v1__spawn_agent\(\{(.*?)\}\)`)
	reMessage     = regexp.MustCompile(`"?message"?:\s*("(?:[^"\\]|\\.)*")`)
	reWait        = regexp.MustCompile(`(?s)tools\.multi_agent_v1__wait_agent\(\{targets:\s*\[(.*?)\],\s*timeout_ms:\s*(\d+)`)
	reTargetOrVar = regexp.MustCompile(`\s*(?:"((?:[^"\\]|\\.)*)"|([\w.]+))\s*(?:,|$)`)
	reAgent       = regexp.MustCompile(`\\"agent_id\\":\\"([0-9a-f-]+)\\"`)
	reYield       = regexp.MustCompile(`"?yield_time_ms"?:\s*(\d+)`)
	reCmd         = regexp.MustCompile(`"?cmd"?:\s*("(?:[^"\\]|\\.)*")`)
)

// modelTurns are the calls the model made in one recorded rollout, in order,
// and its final answer. Codex's model calls its tools from a JS `exec` call
// (tools.exec_command({...}), tools.multi_agent_v1__spawn_agent({...})), so
// the calls are read out of that JS; a call that only looks around (ALL_TOOLS)
// makes none. What the adapter cannot map is an error, never a guess.
func modelTurns(rollout string) (calls []modelCall, err error) {
	var said *string
	var spawned []string // the ids of the sub-agents the model was told of, in order
	nSpawns := 0
	for _, rec := range jsonLines(rollout) {
		p, _ := rec["payload"].(map[string]any)
		if rec["type"] != "response_item" || p == nil {
			continue
		}
		switch {
		case p["type"] == "function_call" || p["type"] == "custom_tool_call" && p["name"] != "exec":
			return nil, fmt.Errorf("the model called %v: the adapter maps only exec", p["name"])
		case p["type"] == "message" && p["role"] == "assistant":
			text := ""
			for _, c := range p["content"].([]any) {
				text += c.(map[string]any)["text"].(string)
			}
			if p["phase"] == "final_answer" {
				calls = append(calls, modelCall{Final: &text})
				said = nil
			} else {
				said = &text
			}
		case p["type"] == "custom_tool_call_output":
			b, _ := json.Marshal(p["output"])
			for _, m := range reAgent.FindAllStringSubmatch(string(b), -1) {
				spawned = append(spawned, m[1])
			}
		case p["type"] == "custom_tool_call":
			js, _ := p["input"].(string)
			for _, m := range reTool.FindAllStringSubmatch(js, -1) {
				switch m[1] {
				case "exec_command", "multi_agent_v1__spawn_agent", "multi_agent_v1__wait_agent":
				default:
					return nil, fmt.Errorf("the model called tools.%s: the adapter maps exec_command and spawn_agent", m[1])
				}
			}
			// the calls in the order the script makes them
			type found struct {
				at   int
				kind string
				m    []string
			}
			var all []found
			for _, k := range []struct {
				kind string
				re   *regexp.Regexp
			}{{"exec", reExec}, {"spawn", reSpawn}, {"wait", reWait}} {
				for _, ix := range k.re.FindAllStringSubmatchIndex(js, -1) {
					m := make([]string, len(ix)/2)
					for g := range m {
						if ix[2*g] >= 0 {
							m[g] = js[ix[2*g]:ix[2*g+1]]
						}
					}
					all = append(all, found{ix[0], k.kind, m})
				}
			}
			sort.Slice(all, func(i, j int) bool { return all[i].at < all[j].at })
			for _, f := range all {
				m := f.m
				switch f.kind {
				case "exec":
					var cmd string
					c := reCmd.FindStringSubmatch(m[1])
					if c == nil || json.Unmarshal([]byte(c[1]), &cmd) != nil {
						return nil, fmt.Errorf("an exec_command whose cmd is not a string literal")
					}
					in := map[string]any{"command": cmd}
					if y := reYield.FindStringSubmatch(m[1]); y != nil {
						n, _ := strconv.Atoi(y[1])
						in["yield_time_ms"] = n
					}
					calls = append(calls, modelCall{Text: said, Name: "Bash", Input: in})
					said = nil
				case "spawn":
					// a spawn with a message, or with no arguments at all (a call the harness refuses)
					in := map[string]any{}
					if strings.TrimSpace(m[1]) != "" {
						var msg string
						mm := reMessage.FindStringSubmatch(m[1])
						if mm == nil || json.Unmarshal([]byte(mm[1]), &msg) != nil {
							return nil, fmt.Errorf("a spawn_agent whose arguments are neither a message literal nor empty")
						}
						in["message"] = msg
					}
					calls = append(calls, modelCall{Text: said, Name: "spawn_agent", Input: in})
					said = nil
					nSpawns++
				case "wait":
					// the targets are agent ids as literals, or a variable holding the id the
					// spawn in the same script was answered with: the sub-agent spawned last
					var targets []string
					for _, e := range reTargetOrVar.FindAllStringSubmatch(m[1], -1) {
						k := -1
						if e[2] != "" {
							k = nSpawns - 1
						} else {
							k = indexOf(spawned, e[1])
						}
						if k < 0 {
							return nil, fmt.Errorf("a wait_agent for an agent the model was not told of")
						}
						targets = append(targets, fmt.Sprintf("AGENT%d", k+1))
					}
					n, _ := strconv.Atoi(m[2])
					calls = append(calls, modelCall{Text: said, Name: "wait_agent", Input: map[string]any{"targets": targets, "timeout_ms": n}})
					said = nil
				}
			}
		}
	}
	return calls, nil
}

func indexOf(list []string, s string) int {
	for i, e := range list {
		if e == s {
			return i
		}
	}
	return -1
}

// scriptFor is the mock script that plays the model's steps, a call or an
// answer each, in the order it took them. The step to play is the number of
// calls it has been answered (function_call_output records) plus the number of
// times a Stop hook has sent it on (hook_prompt records), each of which spent an
// answer. The steps are inside the script, so a sub-agent's script is a file of
// its own. Call ids are unique across the run's scripts (tag), as the real ones
// are: the mock keys a still-running command by its call's id.
func scriptFor(tag string, steps []modelCall) string {
	var lines []string
	for _, c := range steps {
		if c.Final != nil {
			b, _ := json.Marshal(map[string]any{"final": *c.Final})
			lines = append(lines, string(b))
			continue
		}
		content := []any{}
		if c.Text != nil {
			content = append(content, map[string]any{"type": "text", "text": *c.Text})
		}
		content = append(content, map[string]any{"type": "tool_use", "id": "IDPLACE", "name": c.Name, "input": c.Input})
		b, _ := json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{"content": content}})
		lines = append(lines, string(b))
	}
	return fmt.Sprintf(`#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
k=$(grep -c '<hook_prompt' "$A10N_MOCK_SESSION_FILE")
steps=$(cat <<'STEPS_EOF'
%s
STEPS_EOF
)
step=$(printf '%%s\n' "$steps" | sed -n "$((n+k+1))p")
[ -n "$step" ] || step=$(printf '%%s\n' "$steps" | tail -1)
case "$step" in
'{"final":'*)
  text=$(printf '%%s' "$step" | jq -c .final)
  printf '%%s\n' "$(jq -nc --argjson t "$text" '{type:"assistant",message:{content:[{type:"text",text:$t}]}}')" "$(jq -nc --argjson t "$text" '{type:"result",subtype:"success",result:$t}')"
  ;;
*)
  for i in 1 2 3 4 5 6 7 8 9; do
    case "$step" in *AGENT$i*) step=$(printf '%%s' "$step" | sed "s/AGENT$i/$(jq -r 'select(.payload.type=="function_call_output")|.payload.output|try (fromjson|.agent_id) catch empty|select(.!=null)' "$A10N_MOCK_SESSION_FILE" | sed -n "${i}p")/g") ;; esac
  done
  printf '%%s\n' "$step" | sed "s/IDPLACE/call_%s_$n/"
  ;;
esac
`, strings.Join(lines, "\n"), tag)
}

// the command line every replayable recording was made with: the mock is run
// with the equivalent flags, and a recording made another way (a resume, a -c
// override, an output schema) is not replayed by this adapter.
const standardCommand = "codex exec --json --skip-git-repo-check --dangerously-bypass-approvals-and-sandbox --dangerously-bypass-hook-trust -m gpt-5.6-luna"

// Build is the scenario that replays recording rec; the error (an *Unbuildable)
// says what of the recording the adapter cannot reproduce.
func Build(rec Recording) (Scenario, error) {
	cmd := ""
	for _, l := range strings.Split(readFile(filepath.Join(rec.Dir, "run.yaml")), "\n") {
		if v, ok := strings.CutPrefix(l, "command: "); ok {
			cmd = v
		}
	}
	if cmd != standardCommand {
		return Scenario{}, unbuildable(fmt.Errorf("recorded with another command line: %q", cmd))
	}
	entries, _ := os.ReadDir(rec.Setup)
	for _, e := range entries {
		if n := e.Name(); n != "hooks.json" && n != "hook.sh" && n != "prompt.txt" {
			return Scenario{}, unbuildable(fmt.Errorf("the setup has %s, which the adapter does not install", n))
		}
	}
	if rec.Sample == "" {
		return Scenario{}, unbuildable(fmt.Errorf("no sample was recorded"))
	}
	paths, _ := filepath.Glob(filepath.Join(rec.Sample, "transcript", "*.jsonl"))
	if len(paths) == 0 {
		return Scenario{}, unbuildable(fmt.Errorf("no rollout was recorded: the model's turns are unknown"))
	}
	main := ""
	for _, l := range jsonLines(readFile(filepath.Join(rec.Sample, "stream.jsonl"))) {
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
		return Scenario{}, unbuildable(fmt.Errorf("no rollout of the main thread"))
	}
	files := map[string]string{"hook.sh": readFile(filepath.Join(rec.Setup, "hook.sh"))}
	scripts := map[string]string{}
	calls, err := modelTurns(mainRollout)
	if err != nil {
		return Scenario{}, unbuildable(err)
	}
	n := 0
	for i := range calls {
		if calls[i].Name != "spawn_agent" || calls[i].Input["message"] == nil {
			continue
		}
		if n >= len(subs) {
			return Scenario{}, unbuildable(fmt.Errorf("a spawn_agent call with no recorded sub-agent rollout"))
		}
		subCalls, err := modelTurns(subs[n])
		if err != nil {
			return Scenario{}, unbuildable(fmt.Errorf("sub-agent: %w", err))
		}
		name := fmt.Sprintf("sub%d.sh", n)
		scripts[name] = scriptFor(fmt.Sprintf("sub%d", n), subCalls)
		calls[i].Input["script"] = scriptsDir + "/" + name
		n++
	}
	return Scenario{
		HooksJSON: readFile(filepath.Join(rec.Setup, "hooks.json")),
		Files:     files,
		Scripts:   scripts,
		Script:    scriptFor("main", calls),
		Prompt:    strings.TrimSpace(readFile(filepath.Join(rec.Setup, "prompt.txt"))),
	}, nil
}
