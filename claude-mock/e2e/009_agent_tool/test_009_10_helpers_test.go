package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const resultFrame = `{"type":"result","subtype":"success","result":"done","is_error":false}`

// exitHookScript is a hook that drains stdin, writes msg to stderr and/or
// stdout, and exits with code.
func exitHookScript(stdout, stderr string, code int) string {
	b := "cat >/dev/null\n"
	if stdout != "" {
		b += "printf '%s\\n' '" + stdout + "'\n"
	}
	if stderr != "" {
		b += "printf '%s\\n' '" + stderr + "' >&2\n"
	}
	return b + "exit " + string(rune('0'+code%10)) + "\n"
}

// toolScenario runs a Bash tool_use that writes toolFile on its first run and
// finishes on its second. Every run appends a line to runsLog and snapshots the
// session file (what the "model" sees) to sessionCopy.
func toolScenario(t *testing.T, dir, runsLog, sessionCopy, toolFile string) string {
	return toolScenarioCmd(t, dir, runsLog, sessionCopy, "echo ran > "+toolFile)
}

// toolScenarioCmd is toolScenario with an arbitrary Bash command.
func toolScenarioCmd(t *testing.T, dir, runsLog, sessionCopy, cmd string) string {
	return writeScript(t, dir, "scn.sh", `#!/bin/sh
echo run >> "`+runsLog+`"
cp "$A10N_MOCK_SESSION_FILE" "`+sessionCopy+`" 2>/dev/null
if [ "$(wc -l < "`+runsLog+`")" -ge 2 ]; then
  printf '%s\n' '`+resultFrame+`'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_x1","name":"Bash","input":{"command":"`+cmd+`"}}]}}'
`)
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func readOrEmpty(p string) string {
	b, _ := os.ReadFile(p)
	return string(b)
}

// ---- helpers over transcript records ----

func readRecordsFile(t *testing.T, p string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range strings.Split(readOrEmpty(p), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(l), &m), "line: %s", l)
		out = append(out, m)
	}
	return out
}

func transcriptFiles(cfg string) []string {
	var fs []string
	_ = filepath.Walk(cfg, func(p string, i os.FileInfo, err error) error {
		if err == nil && !i.IsDir() && strings.HasSuffix(p, ".jsonl") {
			fs = append(fs, p)
		}
		return nil
	})
	return fs
}

func allRecords(t *testing.T, cfg string) []map[string]any {
	var out []map[string]any
	for _, f := range transcriptFiles(cfg) {
		out = append(out, readRecordsFile(t, f)...)
	}
	return out
}

func allTranscriptText(cfg string) string {
	var b strings.Builder
	for _, f := range transcriptFiles(cfg) {
		b.WriteString(readOrEmpty(f))
	}
	return b.String()
}

func attachmentsOf(recs []map[string]any, typ string) []map[string]any {
	var out []map[string]any
	for _, r := range recs {
		if a, ok := r["attachment"].(map[string]any); ok && a["type"] == typ {
			out = append(out, a)
		}
	}
	return out
}

// modelText is the text of every user-role message record: what the model is told.
func modelText(recs []map[string]any) string {
	var b strings.Builder
	for _, r := range recs {
		if r["type"] == "user" {
			j, _ := json.Marshal(r["message"])
			b.Write(j)
		}
	}
	return b.String()
}

func withoutKeys(m map[string]any, keys ...string) map[string]any {
	c := map[string]any{}
	for k, v := range m {
		c[k] = v
	}
	for _, k := range keys {
		delete(c, k)
	}
	return c
}

// hookWithRaw is a hook printing stdout with NO trailing newline (as the recorded
// hook does), writing stderr, and exiting code.
func hookWithRaw(t *testing.T, dir, stdout, stderr string, code int) string {
	body := "cat >/dev/null\nprintf '%s' '" + stdout + "'\n"
	if stderr != "" {
		body += "printf '%s\\n' '" + stderr + "' >&2\n"
	}
	return writeHook(t, dir, "raw.sh", body+"exit "+string(rune('0'+code))+"\n")
}
