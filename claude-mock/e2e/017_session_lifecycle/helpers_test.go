package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// rec is the subset of a transcript record these tests read.
type rec struct {
	Type              string          `json:"type"`
	Subtype           string          `json:"subtype"`
	UUID              string          `json:"uuid"`
	ParentUUID        *string         `json:"parentUuid"`
	LogicalParentUUID string          `json:"logicalParentUuid"`
	SessionID         string          `json:"sessionId"`
	IsSidechain       bool            `json:"isSidechain"`
	AgentID           string          `json:"agentId"`
	Attachment        map[string]any  `json:"attachment"`
	Message           json.RawMessage `json:"message"`
	ToolUseResult     map[string]any  `json:"-"`
	Raw               string          `json:"-"`
}

func encode(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	return regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(resolved, "-")
}

func transcriptPath(t *testing.T, configDir, projDir, sessionID string) string {
	return filepath.Join(configDir, "projects", encode(t, projDir), sessionID+".jsonl")
}

func readRecs(t *testing.T, path string) []rec {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "transcript %s", path)
	var out []rec
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		var r rec
		require.NoError(t, json.Unmarshal([]byte(l), &r), "line: %s", l)
		var tur struct {
			ToolUseResult json.RawMessage `json:"toolUseResult"`
		}
		_ = json.Unmarshal([]byte(l), &tur)
		_ = json.Unmarshal(tur.ToolUseResult, &r.ToolUseResult) // an object; a string (an error) leaves it nil
		r.Raw = l
		out = append(out, r)
	}
	return out
}

// firstRoot is the first record carrying a uuid and a null parentUuid — the
// transcript's origin as an identity walk finds it.
func firstRoot(recs []rec) (rec, int) {
	for i, r := range recs {
		if r.UUID != "" && r.ParentUUID == nil {
			return r, i
		}
	}
	return rec{}, -1
}

func write(t *testing.T, path, body string, mode os.FileMode) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), mode))
	return path
}

// settings writes .claude/settings.json in dir wiring each event to a command.
func settings(t *testing.T, dir string, hooks map[string]string) {
	t.Helper()
	h := map[string]any{}
	for ev, cmd := range hooks {
		h[ev] = []any{map[string]any{"matcher": "*", "hooks": []any{map[string]any{"type": "command", "command": cmd}}}}
	}
	b, err := json.Marshal(map[string]any{"hooks": h})
	require.NoError(t, err)
	write(t, filepath.Join(dir, ".claude", "settings.json"), string(b), 0o644)
}

// payloadLogger is a hook that appends its stdin payload to log, one per line.
func payloadLogger(t *testing.T, dir, name, log, extra string) string {
	t.Helper()
	return write(t, filepath.Join(dir, name), "#!/bin/sh\nIN=$(cat)\nprintf '%s\\n' \"$IN\" >> "+log+"\n"+extra+"\n", 0o755)
}

// payloads reads the logged payloads.
func payloads(t *testing.T, log string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(log)
	require.NoError(t, err)
	var out []map[string]any
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(l), &m), "payload: %s", l)
		out = append(out, m)
	}
	return out
}

// script writes a scenario that emits each line once, in order, one per turn,
// then a final result. A turn is "done" once its marker is in the transcript.
func script(t *testing.T, dir, name string, lines ...string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("#!/bin/sh\nF=\"$A10N_MOCK_SESSION_FILE\"\n")
	for i, l := range lines {
		marker := "turn-" + name + "-" + string(rune('a'+i))
		l = strings.Replace(l, "@MARK@", marker, 1)
		b.WriteString("if ! grep -q '" + marker + "' \"$F\" 2>/dev/null; then\ncat <<'JSONL'\n" + l + "\nJSONL\nexit 0\nfi\n")
	}
	b.WriteString(`echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"done"}]}}'` + "\n")
	b.WriteString(`echo '{"type":"result","subtype":"success","result":"done"}'` + "\n")
	return write(t, filepath.Join(dir, name+".sh"), b.String(), 0o755)
}

func toolUse(id, name, input string) string {
	return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"` + id + `@MARK@","name":"` + name + `","input":` + input + `}]}}`
}
