package replay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Scenario is what the mock is given to replay a recording: the run's own
// setup (its hooks.json, its hook script, its prompt) and the scenario script
// that makes the model's calls, in the format the mock takes of any scenario.
type Scenario struct {
	HooksJSON string
	// Files are written into the repository (hook.sh, and a sub-agent's script).
	Files  map[string]string
	Script string
	Prompt string
}

// Unbuildable says what of a recording the adapter cannot reproduce yet.
type Unbuildable struct{ Reason string }

func (u *Unbuildable) Error() string { return u.Reason }

func unbuildable(err error) error { return &Unbuildable{Reason: err.Error()} }

// modelCall is one tool call the model made, in the mock's script vocabulary.
type modelCall struct {
	Text  *string        `json:"text,omitempty"` // what the model said just before the call, if it said anything
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

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
// the calls are read out of that JS; a call that only looks around (ALL_TOOLS)
// makes none. What the adapter cannot map is an error, never a guess.
func modelTurns(rollout string) (calls []modelCall, final string, err error) {
	var said *string
	for _, rec := range jsonLines(rollout) {
		p, _ := rec["payload"].(map[string]any)
		if rec["type"] != "response_item" || p == nil {
			continue
		}
		switch {
		case p["type"] == "function_call" || p["type"] == "custom_tool_call" && p["name"] != "exec":
			return nil, "", fmt.Errorf("the model called %v: the adapter maps only exec", p["name"])
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
					return nil, "", fmt.Errorf("the model called tools.%s: the adapter maps exec_command and spawn_agent", m[1])
				}
			}
			for _, m := range reExec.FindAllStringSubmatch(js, -1) {
				var cmd string
				c := reCmd.FindStringSubmatch(m[1])
				if c == nil || json.Unmarshal([]byte(c[1]), &cmd) != nil {
					return nil, "", fmt.Errorf("an exec_command whose cmd is not a string literal")
				}
				in := map[string]any{"command": cmd}
				if y := reYield.FindStringSubmatch(m[1]); y != nil {
					n, _ := strconv.Atoi(y[1])
					in["yield_time_ms"] = n
				}
				calls = append(calls, modelCall{Text: said, Name: "Bash", Input: in})
				said = nil
			}
			for _, m := range reSpawn.FindAllStringSubmatch(js, -1) {
				var msg string
				if json.Unmarshal([]byte(m[1]), &msg) != nil {
					return nil, "", fmt.Errorf("a spawn_agent whose message is not a string literal")
				}
				calls = append(calls, modelCall{Text: said, Name: "spawn_agent", Input: map[string]any{"message": msg}})
				said = nil
			}
		}
	}
	return calls, final, nil
}

// scriptFor is the mock script that makes the given calls, one per model turn,
// then answers with final. The calls are inside it, so a sub-agent's script is
// a file of its own. Call ids are unique across the run's scripts (tag), as the
// real ones are: the mock keys a still-running command by its call's id.
func scriptFor(tag string, calls []modelCall, final string) string {
	var lines []string
	for _, c := range calls {
		content := []any{}
		if c.Text != nil {
			content = append(content, map[string]any{"type": "text", "text": *c.Text})
		}
		content = append(content, map[string]any{"type": "tool_use", "id": "IDPLACE", "name": c.Name, "input": c.Input})
		b, _ := json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{"content": content}})
		lines = append(lines, string(b))
	}
	fin, _ := json.Marshal(final)
	return fmt.Sprintf(`#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
call=$(sed -n "$((n+1))p" <<'CALLS_EOF'
%s
CALLS_EOF
)
if [ -n "$call" ]; then
  printf '%%s\n' "$call" | sed "s/IDPLACE/call_%s_$n/"
  exit 0
fi
text='%s'
printf '%%s\n' "$(jq -nc --argjson t "$text" '{type:"assistant",message:{content:[{type:"text",text:$t}]}}')" "$(jq -nc --argjson t "$text" '{type:"result",subtype:"success",result:$t}')"
`, strings.Join(lines, "\n"), tag, strings.ReplaceAll(string(fin), "'", `'\''`))
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
	calls, final, err := modelTurns(mainRollout)
	if err != nil {
		return Scenario{}, unbuildable(err)
	}
	n := 0
	for i := range calls {
		if calls[i].Name != "spawn_agent" {
			continue
		}
		if n >= len(subs) {
			return Scenario{}, unbuildable(fmt.Errorf("a spawn_agent call with no recorded sub-agent rollout"))
		}
		subCalls, subFinal, err := modelTurns(subs[n])
		if err != nil {
			return Scenario{}, unbuildable(fmt.Errorf("sub-agent: %w", err))
		}
		name := fmt.Sprintf("sub%d.sh", n)
		files[name] = scriptFor(fmt.Sprintf("sub%d", n), subCalls, subFinal)
		calls[i].Input["script"] = name
		n++
	}
	return Scenario{
		HooksJSON: readFile(filepath.Join(rec.Setup, "hooks.json")),
		Files:     files,
		Script:    scriptFor("main", calls, final),
		Prompt:    strings.TrimSpace(readFile(filepath.Join(rec.Setup, "prompt.txt"))),
	}, nil
}
