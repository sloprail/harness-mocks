package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// recordedSample is the newest sealed sample of a recorded run
// (claude-mock/snapshots/runs/<run>/samples/<ts>).
func recordedSample(t *testing.T, run string) string {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join("..", "..", "snapshots", "runs", run, "samples", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, dirs, "run %s has no sample", run)
	sort.Strings(dirs)
	return dirs[len(dirs)-1]
}

// recordedHooks are the hook payloads the real claude gave a recorded run's
// hook command, in order.
func recordedHooks(t *testing.T, run string) []map[string]any {
	t.Helper()
	log := filepath.Join(recordedSample(t, run), "payloads.jsonl")
	return payloads(t, log)
}

// recordedAgentPost is the PostToolUse payload of the n-th Agent call (0-based,
// in firing order) of a recorded run.
func recordedAgentPost(t *testing.T, run string, n int) map[string]any {
	t.Helper()
	return agentPosts(recordedHooks(t, run))[n]
}

// agentPosts are the Agent PostToolUse payloads among hook payloads.
func agentPosts(all []map[string]any) []map[string]any {
	var out []map[string]any
	for _, p := range all {
		if p["hook_event_name"] == "PostToolUse" && p["tool_name"] == "Agent" {
			out = append(out, p)
		}
	}
	return out
}

// recordedAgentResultText is the text of the first Agent tool_result in a
// recorded run's stream: what the parent model was handed.
func recordedAgentResultText(t *testing.T, run string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(recordedSample(t, run), "stream.jsonl"))
	require.NoError(t, err)
	for _, l := range strings.Split(string(raw), "\n") {
		var m struct {
			Type    string `json:"type"`
			Message struct {
				Content []struct {
					Type    string `json:"type"`
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(l), &m) != nil || m.Type != "user" {
			continue
		}
		for _, b := range m.Message.Content {
			if b.Type == "tool_result" && len(b.Content) == 1 {
				return b.Content[0].Text
			}
		}
	}
	t.Fatalf("run %s has no tool_result", run)
	return ""
}

// keysOf are a payload's keys, sorted.
func keysOf(m map[string]any) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// replyScript is a sub-agent scenario that replies with text and nothing else.
func replyScript(t *testing.T, dir, name, text string) string {
	t.Helper()
	msg, err := json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{
		"role": "assistant", "stop_reason": "end_turn", "content": []any{map[string]any{"type": "text", "text": text}}}})
	require.NoError(t, err)
	res, err := json.Marshal(map[string]any{"type": "result", "subtype": "success", "result": text})
	require.NoError(t, err)
	return write(t, filepath.Join(dir, name+".sh"), "#!/bin/sh\ncat <<'JSONL'\n"+string(msg)+"\n"+string(res)+"\nJSONL\n", 0o755)
}

// callThenReply is a scenario whose first turn emits the tool_use lines (each
// toolUse id gets "m<i>" appended) and whose next turn replies with text.
func callThenReply(t *testing.T, dir, name, text string, calls ...string) string {
	t.Helper()
	msg, err := json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{
		"role": "assistant", "stop_reason": "end_turn", "content": []any{map[string]any{"type": "text", "text": text}}}})
	require.NoError(t, err)
	res, err := json.Marshal(map[string]any{"type": "result", "subtype": "success", "result": text})
	require.NoError(t, err)
	var b strings.Builder
	b.WriteString("#!/bin/sh\nF=\"$A10N_MOCK_SESSION_FILE\"\n")
	for i, c := range calls {
		mark := "m" + string(rune('a'+i)) + name
		b.WriteString("if ! grep -q '" + mark + "' \"$F\" 2>/dev/null; then\ncat <<'JSONL'\n" + strings.Replace(c, "@MARK@", mark, 1) + "\nJSONL\nexit 0\nfi\n")
	}
	b.WriteString("cat <<'JSONL'\n" + string(msg) + "\n" + string(res) + "\nJSONL\n")
	return write(t, filepath.Join(dir, name+".sh"), b.String(), 0o755)
}

// gitIn runs git in dir.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)...).CombinedOutput()
	require.NoError(t, err, string(out))
}

// allHooks wires every event the recorded runs log to one command.
func allHooks(h string) map[string]string {
	m := map[string]string{}
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "PostToolUseFailure", "SubagentStart", "SubagentStop", "Stop", "SessionEnd"} {
		m[ev] = h
	}
	return m
}

// hookLabels name each payload by its event, its tool and whether a sub-agent
// issued it: the order the hooks fired in, as the recordings show it.
func hookLabels(all []map[string]any) []string {
	var out []string
	for _, p := range all {
		l, _ := p["hook_event_name"].(string)
		if tn, ok := p["tool_name"].(string); ok {
			l += ":" + tn
		}
		if _, ok := p["agent_id"]; ok && !strings.HasPrefix(l, "Subagent") {
			l += "[in sub-agent]"
		}
		out = append(out, l)
	}
	return out
}

// recordedNotificationSummary is the summary of the last task_notification
// frame in a recorded run's stream.
func recordedNotificationSummary(t *testing.T, run string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(recordedSample(t, run), "stream.jsonl"))
	require.NoError(t, err)
	summary, found := "", false
	for _, l := range strings.Split(string(raw), "\n") {
		var m struct {
			Subtype, Summary string
		}
		if json.Unmarshal([]byte(l), &m) == nil && m.Subtype == "task_notification" {
			summary, found = m.Summary, true
		}
	}
	require.True(t, found, "run %s has no task_notification", run)
	return summary
}

// lastResultStats is the subagent_stats of the last result frame in a stream.
func lastResultStats(t *testing.T, stream string) map[string]any {
	t.Helper()
	var stats map[string]any
	for _, l := range strings.Split(stream, "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil && m["type"] == "result" {
			stats, _ = m["subagent_stats"].(map[string]any)
		}
	}
	require.NotNil(t, stats, "the last result frame carries subagent_stats")
	return stats
}

// recordedResultStats is the subagent_stats the real claude's last result frame of
// a recorded run carried.
func recordedResultStats(t *testing.T, run string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(recordedSample(t, run), "stream.jsonl"))
	require.NoError(t, err)
	return lastResultStats(t, string(raw))
}
