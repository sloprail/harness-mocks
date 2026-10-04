package e2e

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	rp "github.com/sloprail/harness-mocks/internal/replay"
)

// The codex adapter of the generic replay. For every recorded run in
// snapshots/runs it turns the recording into the mock's script (the model's
// turns, read from the recorded rollouts), runs the mock on the run's own
// setup, and has internal/replay compare the mock's whole event stream and
// hook payloads with the recording's. The table is the recording folders
// themselves: a recording without a replay, or a replay without a recording,
// cannot exist.

// printScript prints the script generated for each replayed run, for debugging:
//
//	go test ./codex-mock/e2e/... -run 'TestGeneratedReplay/<run>$' -v -args -replay.print-script
var printScript = flag.Bool("replay.print-script", false, "print the scenario scripts the replay generates")

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

// buildReplay is the scenario that replays recording rec; the error says what
// of the recording the adapter cannot reproduce.
func buildReplay(t *testing.T, rec recording) (scenario, error) {
	t.Helper()
	cmd := ""
	for _, l := range strings.Split(readFile(t, filepath.Join(filepath.Dir(rec.setup), "run.yaml")), "\n") {
		if v, ok := strings.CutPrefix(l, "command: "); ok {
			cmd = v
		}
	}
	if cmd != standardCommand {
		return scenario{}, fmt.Errorf("recorded with another command line: %q", cmd)
	}
	entries, _ := os.ReadDir(rec.setup)
	for _, e := range entries {
		if n := e.Name(); n != "hooks.json" && n != "hook.sh" && n != "prompt.txt" {
			return scenario{}, fmt.Errorf("the setup has %s, which the adapter does not install", n)
		}
	}
	if rec.sample == "" {
		return scenario{}, fmt.Errorf("no sample was recorded")
	}
	paths, _ := filepath.Glob(filepath.Join(rec.sample, "transcript", "*.jsonl"))
	if len(paths) == 0 {
		return scenario{}, fmt.Errorf("no rollout was recorded: the model's turns are unknown")
	}
	main := ""
	for _, l := range jsonLines(readFile(t, filepath.Join(rec.sample, "stream.jsonl"))) {
		if l["type"] == "thread.started" {
			main, _ = l["thread_id"].(string)
		}
	}
	var mainRollout string
	var subs []string
	for _, p := range paths {
		if strings.Contains(p, main) {
			mainRollout = readFile(t, p)
		} else {
			subs = append(subs, readFile(t, p))
		}
	}
	if mainRollout == "" {
		return scenario{}, fmt.Errorf("no rollout of the main thread")
	}
	files := map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))}
	calls, final, err := modelTurns(mainRollout)
	if err != nil {
		return scenario{}, err
	}
	n := 0
	for i := range calls {
		if calls[i].Name != "spawn_agent" {
			continue
		}
		if n >= len(subs) {
			return scenario{}, fmt.Errorf("a spawn_agent call with no recorded sub-agent rollout")
		}
		subCalls, subFinal, err := modelTurns(subs[n])
		if err != nil {
			return scenario{}, fmt.Errorf("sub-agent: %w", err)
		}
		name := fmt.Sprintf("sub%d.sh", n)
		files[name] = scriptFor(fmt.Sprintf("sub%d", n), subCalls, subFinal)
		calls[i].Input["script"] = name
		n++
	}
	return scenario{
		HooksJSON:   readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files:       files,
		Script:      scriptFor("main", calls, final),
		Prompt:      strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))),
		BypassTrust: true,
	}, nil
}

// replayRules are what a recording and a replay of it may differ in, and why:
// no capability cell is about any of it.
func replayRules(got result) rp.Rules {
	re := regexp.MustCompile
	return rp.Rules{
		DropKeys: []string{
			"usage",                  // token counts: the mock has no model
			"model",                  // gpt-5.6-luna against the mock's name
			"transcript_path",        // where the rollout is kept: a path
			"cwd",                    // the run's directory: a path
			"turn_id", "tool_use_id", // ids that differ in every run
			"wall_time_seconds", "duration_ms", // timings
			"agent_transcript_path",
			"script", // the mock's own spawn_agent parameter: the sub-agent's script
		},
		Rewrite: map[string]func(string) string{
			// a `ps` listing is the host's: only the job's own processes are the behaviour
			"aggregated_output": jobProcesses,
			"tool_response":     jobProcesses,
			// how the shell was invoked is the machine's: the command is what the model asked for
			"command": shellInner,
		},
		Scrub: []rp.Scrub{
			{Re: re(regexp.QuoteMeta(got.Repo)), With: "<RUN>"},
			{Re: re(regexp.QuoteMeta(got.Tmp)), With: "<TMP>"},
		},
		IDs: []*regexp.Regexp{re(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)}, // thread and session ids
	}
}

// shellInner is the command inside `/bin/<shell> -c[l] <command>`, whichever
// way the command was quoted (double quotes as codex writes it, single quotes
// as the mock does, none for a single word); a line that is not such an
// invocation is returned as it is.
func shellInner(line string) string {
	m := regexp.MustCompile(`^/bin/\w+ -l?c (.*)$`).FindStringSubmatch(line)
	if m == nil {
		return line
	}
	arg := m[1]
	switch {
	case strings.HasPrefix(arg, `"`) && strings.HasSuffix(arg, `"`):
		return strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(arg[1 : len(arg)-1])
	case strings.HasPrefix(arg, "'") && strings.HasSuffix(arg, "'"):
		return strings.ReplaceAll(arg[1:len(arg)-1], `'\''`, "'")
	}
	return arg
}

// jobProcesses reduces the output of `ps ax` to the processes the replayed
// command started (`sleep N`, or a shell running it), without pids, ttys and
// times: the rest of the listing is the machine's (the test runner, the
// harness, other sessions), and not what any capability cell is about. Text
// that is not a `ps` listing is returned as it is.
func jobProcesses(text string) string {
	ps := regexp.MustCompile(`^\s*\d+\s+\S+\s+\S+\s+\d+:\d+(?:\.\d+)?\s+(.*)$`)
	job := regexp.MustCompile(`^(?:/bin/\w+ -c )?sleep \d+`)
	var jobs []string
	listing := false
	for _, l := range strings.Split(text, "\n") {
		if m := ps.FindStringSubmatch(l); m != nil {
			listing = true
			if job.MatchString(m[1]) {
				jobs = append(jobs, "<job> "+regexp.MustCompile(`^/bin/\w+ `).ReplaceAllString(m[1], "<SHELL> "))
			}
		}
	}
	if !listing {
		return text
	}
	return "<ps: " + strings.Join(jobs, " | ") + ">"
}

// recordingOf is the latest sample of a run, without reading the calls out of
// its hook payloads (loadRecording does, and not every run's calls are shell commands).
func recordingOf(t *testing.T, name string) recording {
	t.Helper()
	samples, err := filepath.Glob(filepath.Join(runsDir, name, "samples", "*"))
	require.NoError(t, err)
	if len(samples) == 0 {
		return recording{setup: filepath.Join(runsDir, name, "setup")}
	}
	return recording{setup: filepath.Join(runsDir, name, "setup"), sample: samples[len(samples)-1]}
}

// replayDiff replays the recording and returns what differs from it; empty is
// a green replay. A recording the adapter cannot build is an error.
func replayDiff(t *testing.T, rec recording) (diff string, unbuildable error) {
	t.Helper()
	s, err := buildReplay(t, rec)
	if err != nil {
		return "", err
	}
	if *printScript {
		t.Logf("scenario script (main):\n%s", s.Script)
		for name, body := range s.Files {
			if name != "hook.sh" {
				t.Logf("scenario script (%s):\n%s", name, body)
			}
		}
	}
	got := execMock(t, s)
	if got.Code != 0 {
		return fmt.Sprintf("the mock exited %d: %s", got.Code, got.Stderr), nil
	}
	// one canonicalisation per side, the event stream first: it names the ids in a fixed order
	rules := replayRules(got)
	wantC, gotC := rp.New(rules), rp.New(rules)
	wantStream := wantC.Lines(jsonLines(readFile(t, filepath.Join(rec.sample, "stream.jsonl"))))
	gotStream := gotC.Lines(got.stream())
	wantHooks := wantC.Lines(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))))
	gotHooks := gotC.Lines(got.hookLog())
	// hooks of one event run at the same time, so the order they log in is not the behaviour
	return rp.Diff("event stream", wantStream, gotStream) +
		rp.Diff("hook payloads", rp.Sorted(wantHooks), rp.Sorted(gotHooks)), nil
}

// Every recorded run, replayed by one generic test: the mock is given the
// run's own setup and the model's own turns, and its whole event stream and
// hook payloads are compared with what the real codex left. A run that does
// not replay green is listed in notReplaying with its reason.
func TestGeneratedReplay(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join(runsDir, "*"))
	require.NoError(t, err)
	var names []string
	for _, d := range dirs {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			names = append(names, filepath.Base(d))
		}
	}
	sort.Strings(names)
	for name := range notReplaying {
		if _, err := os.Stat(filepath.Join(runsDir, name)); err != nil {
			t.Errorf("notReplaying lists %s, which has no recording: remove the entry", name)
		}
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			diff, unbuildable := replayDiff(t, recordingOf(t, name))
			reason, listed := notReplaying[name]
			switch {
			case unbuildable != nil && listed:
				t.Skipf("not replaying: %s", reason)
			case unbuildable != nil:
				t.Errorf("not replayed (%v): list it in notReplaying with the reason, or extend the adapter", unbuildable)
			case diff == "" && listed && strings.HasPrefix(reason, "flaky:"):
			case diff == "" && listed:
				t.Errorf("replays green: remove it from notReplaying (was: %s)", reason)
			case diff != "" && listed:
				t.Skipf("not replaying: %s\n%s", reason, diff)
			case diff != "":
				t.Error(diff)
			}
		})
	}
}
