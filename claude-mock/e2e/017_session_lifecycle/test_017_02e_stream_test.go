package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_28_RefusedNotificationStartsNoTurn: when UserPromptSubmit refuses a
// task notification, the notification is not written and no turn runs for it;
// the session ends.
// sr:proves task-notifications/claude
// sr:proves user-prompt-submit-hook/claude
func TestT017_28_RefusedNotificationStartsNoTurn(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, `if printf '%s' "$IN" | grep -q 'task-notification'; then echo refused 1>&2; exit 2; fi`)
	settings(t, dir, map[string]string{"UserPromptSubmit": h, "Stop": h})
	sub := write(t, filepath.Join(dir, "sub.sh"), `#!/bin/sh
sleep 1
printf '%s\n' '{"type":"result","subtype":"success","result":"R"}'
`, 0o755)
	sc := script(t, dir, "s",
		toolUse("ag1", "Agent", `{"prompt":"go","description":"bg","script":"`+sub+`","run_in_background":true}`),
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "rn-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	raw, err := os.ReadFile(transcriptPath(t, cfg, dir, "rn-1"))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "<task-notification>", "a refused notification is not written")
	var stops int
	for _, p := range payloads(t, log) {
		if p["hook_event_name"] == "Stop" {
			stops++
		}
	}
	assert.Equal(t, 1, stops, "no turn ran for it")
}

// TestT017_29_FailingBash: a foreground Bash that exits non-zero is answered
// "Exit code N\n<stdout>\n<stderr>" (is_error) with toolUseResult "Error:
// <that>", and fires PostToolUseFailure {error, is_interrupt, duration_ms}
// instead of PostToolUse; the next Bash, which succeeds, fires PostToolUse
// only (recorded: snapshots/runs/bashfail, the same two commands).
// sr:docs https://code.claude.com/docs/en/hooks#posttoolusefailure-input
// sr:proves bash-tool-result/claude
// sr:proves tool-failure-hook/claude
// sr:proves posttooluse-payload/claude
func TestT017_29_FailingBash(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"PostToolUse": h, "PostToolUseFailure": h})
	sc := script(t, dir, "s",
		toolUse("b1", "Bash", `{"command":"echo OUT-LINE; echo ERR-LINE >&2; exit 3"}`),
		toolUse("b2", "Bash", `{"command":"true"}`))
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "bf-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	block, _ := toolResultOf(t, readRecs(t, transcriptPath(t, cfg, dir, "bf-1")), "b1turn-s-a")
	assert.Equal(t, "Exit code 3\nOUT-LINE\nERR-LINE", block["content"])
	assert.Equal(t, true, block["is_error"])
	raw, _ := os.ReadFile(transcriptPath(t, cfg, dir, "bf-1"))
	assert.Contains(t, string(raw), `"toolUseResult":"Error: Exit code 3\nOUT-LINE\nERR-LINE"`)
	ps := payloads(t, log)
	require.Len(t, ps, 2)
	assert.Equal(t, "PostToolUseFailure", ps[0]["hook_event_name"])
	assert.Equal(t, "Exit code 3\nOUT-LINE\nERR-LINE", ps[0]["error"])
	assert.Equal(t, false, ps[0]["is_interrupt"])
	d, ok := ps[0]["duration_ms"].(float64)
	assert.True(t, ok && d >= 0, "duration_ms is a number of milliseconds: %v", ps[0]["duration_ms"])
	assert.NotContains(t, ps[0], "tool_response")
	assert.Equal(t, "PostToolUse", ps[1]["hook_event_name"])
	assert.Equal(t, "true", ps[1]["tool_input"].(map[string]any)["command"])
}

// TestT017_29b_FailingRead: a Read of a file that does not exist is a tool
// that ran and failed: the agent gets "File does not exist. Note: your
// current working directory is <cwd>." (is_error) and PostToolUseFailure
// fires with that text as error, not PostToolUse (recorded:
// snapshots/runs/tool-errors).
// sr:docs https://code.claude.com/docs/en/hooks#posttoolusefailure
// sr:proves tool-failure-hook/claude
// sr:proves file-tools/claude
func TestT017_29b_FailingRead(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"PostToolUse": h, "PostToolUseFailure": h})
	sc := script(t, dir, "s", toolUse("r1", "Read", `{"file_path":"nonexistent-dir/missing.txt"}`))
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "rf-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	want := "File does not exist. Note: your current working directory is " + real + "."
	block, _ := toolResultOf(t, readRecs(t, transcriptPath(t, cfg, dir, "rf-1")), "r1turn-s-a")
	assert.Equal(t, want, block["content"])
	assert.Equal(t, true, block["is_error"])
	ps := payloads(t, log)
	require.Len(t, ps, 1)
	assert.Equal(t, "PostToolUseFailure", ps[0]["hook_event_name"])
	assert.Equal(t, "Read", ps[0]["tool_name"])
	assert.Equal(t, want, ps[0]["error"])
}

// TestT017_30_SubagentMetaSidecars: every sub-agent's .meta.json has the
// fields claude 2.1.282 writes — spawnDepth, requestShape, requestNonInteractive;
// parentAgentId for a nested one; model when the call names one; and for an
// isolated one worktreePath, spawnedWithWorktree and the worktree-agent-<id>
// branch it really creates (fixture evidence/meta, 626 real sidecars).
// sr:proves nested-subagents/claude
// sr:proves subagent-transcripts/claude
// sr:proves subagent-worktree-isolation/claude
func TestT017_30_SubagentMetaSidecars(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	require.NoError(t, exec.Command("git", "init", "-q", dir).Run())
	require.NoError(t, exec.Command("git", "-C", dir, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "i").Run())
	leaf := script(t, dir, "leaf")
	// the isolated sub-agent leaves a file in its worktree, so the worktree stays (a clean one is removed)
	worker := script(t, dir, "worker", toolUse("w1", "Bash", `{"command":"touch left-behind.txt"}`))
	outer := script(t, dir, "outer", toolUse("in1", "Agent", `{"prompt":"p","description":"inner","script":"`+leaf+`"}`))
	sc := script(t, dir, "s",
		toolUse("o1", "Agent", `{"prompt":"p","description":"outer","script":"`+outer+`","model":"haiku"}`),
		toolUse("i1", "Agent", `{"prompt":"p","description":"iso","isolation":"worktree","script":"`+worker+`"}`),
		toolUse("g1", "Agent", `{"prompt":"p","description":"bg","run_in_background":true,"script":"`+leaf+`"}`),
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "mt-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	files, err := filepath.Glob(filepath.Join(strings.TrimSuffix(transcriptPath(t, cfg, dir, "mt-1"), ".jsonl"), "subagents", "*.meta.json"))
	require.NoError(t, err)
	byDesc := map[string]map[string]any{}
	ids := map[string]string{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))
		byDesc[m["description"].(string)] = m
		ids[m["description"].(string)] = strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "agent-"), ".meta.json")
	}
	require.Len(t, byDesc, 4)
	base := func(desc, shape string, depth int) map[string]any {
		return map[string]any{"agentType": "general-purpose", "description": desc, "spawnDepth": float64(depth),
			"requestShape": shape, "requestNonInteractive": true}
	}
	o := base("outer", "foreground", 1)
	o["model"], o["toolUseId"] = "haiku", byDesc["outer"]["toolUseId"]
	assert.Equal(t, o, byDesc["outer"])
	in := base("inner", "foreground", 2)
	in["parentAgentId"], in["toolUseId"] = ids["outer"], byDesc["inner"]["toolUseId"]
	assert.Equal(t, in, byDesc["inner"])
	g := base("bg", "background", 1)
	g["toolUseId"] = byDesc["bg"]["toolUseId"]
	assert.Equal(t, g, byDesc["bg"])
	iso := byDesc["iso"]
	resolved, _ := filepath.EvalSymlinks(dir)
	assert.Equal(t, filepath.Join(resolved, ".claude", "worktrees", "agent-"+ids["iso"]), iso["worktreePath"])
	assert.Equal(t, true, iso["spawnedWithWorktree"])
	assert.Equal(t, "worktree-agent-"+ids["iso"], iso["worktreeBranch"])
	branches, err := exec.Command("git", "-C", dir, "branch", "--list", "worktree-agent-*").Output()
	require.NoError(t, err)
	assert.Contains(t, string(branches), "worktree-agent-"+ids["iso"], "the branch really exists")
}

// TestT017_31_StreamStaysParseableUnderLoad: a background sub-agent streams
// task frames for its own Bash calls while the main turn streams a flood of
// lines, and every stdout line must parse as JSON — a frame written between
// another line and its newline breaks both (it did with the old two-Write
// writeStreamLine).
//
// The overlap between the flood and the sub-agent's frames is a RENDEZVOUS,
// not a race to finish before the other side does: the flood writes its
// output in small chunks, and after each chunk it BLOCKS until the
// sub-agent's own call counter has advanced past where it started — i.e.
// until at least one more Bash tool_use (and so at least one more pair of
// owned_by_subagent frames) has been processed by the mock. This makes the
// interleaving depend only on the sub-agent script's counter file being
// updated, not on relative process speed, so a slow host runs more, smaller
// rounds of "chunk, then wait" instead of one round that might race to
// completion before any frame lands. Each wait is itself bounded (flood.sh's
// STALL_LIMIT): a sub-agent that never advances fails the test fast with a
// clear message instead of hanging the whole package for Go's default
// 10-minute test timeout.
//
// This test's own detection of the regression is necessarily probabilistic —
// it depends on the OS scheduler actually interleaving two real processes'
// writes to the shared pipe within a chunk's write window, not on a
// deterministic code path. TestStreamLinesStayWholeUnderConcurrentFrames (in
// background_test.go), which exercises writeStreamLine directly from many
// goroutines under one process, is the primary, fully deterministic
// regression guard; this test is the end-to-end confirmation that the same
// guarantee survives the real CLI/turn-loop/background-agent boundary.
// Measured (this revision, -race, 25 rounds each): against a build with
// writeStreamLine reverted to two separate Write calls, this test failed
// 25/25 runs; against the fix, it passed 25/25 runs. Scheduler-dependence
// means neither number is a guarantee for all future hosts/loads, which is
// exactly why TestStreamLinesStayWholeUnderConcurrentFrames — not this
// test — is the deterministic guard; treat a rare flake here as a scheduler
// artifact to re-run, not a first alarm.
func TestT017_31_StreamStaysParseableUnderLoad(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	done, calls := filepath.Join(dir, "flood-done"), filepath.Join(dir, "calls")
	// The sub-agent counts its own turns in $calls (one Bash tool_use per
	// turn) and stops once $done exists.
	sub := write(t, filepath.Join(dir, "sub.sh"), `#!/bin/sh
if [ -f `+done+` ]; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"sub done"}'
  exit 0
fi
N=$(cat `+calls+` 2>/dev/null || echo 0); N=$((N+1)); echo $N > `+calls+`
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"sb'$N'","name":"Bash","input":{"command":"true","description":"step '$N'"}}]}}'
`, 0o755)
	// The flood writes ROUNDS lines chunk (a "line-<round>-<i>" burst), then
	// waits for $calls to read a value strictly greater than it was before
	// this chunk. So every chunk boundary is a real rendezvous with a
	// completed sub-agent turn, not a fixed sleep a fast or slow host could
	// race past. The wait is bounded (STALL_LIMIT * 0.005s ≈ 15s): if the
	// sub-agent ever stalls, the flood exits 1 with a clear message rather
	// than hanging the whole test binary for its default 10-minute timeout —
	// a stall here is itself a test failure worth seeing fast, not a reason
	// to block indefinitely.
	flood := write(t, filepath.Join(dir, "flood.sh"), `#!/bin/sh
CHUNK=200
ROUNDS=600
STALL_LIMIT=3000
r=0
while [ $r -lt $ROUNDS ]; do
  i=0
  while [ $i -lt $CHUNK ]; do
    printf '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"line-%d-%d xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}]}}
' "$r" "$i"
    i=$((i+1))
  done
  before=$(cat `+calls+` 2>/dev/null || echo 0)
  after=$before
  stalled=0
  while [ "$after" = "$before" ]; do
    stalled=$((stalled+1))
    if [ $stalled -ge $STALL_LIMIT ]; then
      echo "flood.sh: the sub-agent's call counter did not advance past $before after $STALL_LIMIT polls — stalled, giving up" 1>&2
      exit 1
    fi
    sleep 0.005
    after=$(cat `+calls+` 2>/dev/null || echo 0)
  done
  r=$((r+1))
done
`, 0o755)
	sc := write(t, filepath.Join(dir, "s.sh"), `#!/bin/sh
F="$A10N_MOCK_SESSION_FILE"
if ! grep -q '"tool_use_id":"ag1"' "$F"; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"ag1","name":"Agent","input":{"prompt":"go","description":"busy","script":"`+sub+`","run_in_background":true}}]}}'
  exit 0
fi
if [ ! -f `+done+` ]; then
  sh `+flood+`
  touch `+done+`
fi
printf '%s\n' '{"type":"result","subtype":"success","result":"done"}'
`, 0o755)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "load-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, "exit code (a non-zero code including a flood stall is reported in the output below):\n%.2000s", out)
	var owned, bad, during int
	floodSeen, floodOver := false, false
	for i, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if !strings.HasPrefix(l, "{") {
			continue // the mock's own stderr diagnostics share this capture
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			bad++
			if bad <= 3 {
				t.Errorf("line %d does not parse: %.160s", i, l)
			}
			continue
		}
		if strings.Contains(l, "line-0-0 x") {
			floodSeen = true
		}
		if strings.Contains(l, "line-599-199 x") {
			floodOver = true
		}
		if m["owned_by_subagent"] == true {
			owned++
			if floodSeen && !floodOver {
				during++
			}
		}
	}
	t.Logf("%d owned-Bash frames, %d of them streamed during the flood", owned, during)
	assert.Zero(t, bad, "unparseable stream lines")
	// Every completed rendezvous round (bar the very first and very last,
	// which can fall outside the flood's own start/end markers) forces one
	// interleaving point, so this floor is met deterministically rather than
	// by how fast the host happens to run.
	assert.Greater(t, during, 100, "the sub-agent's frames overlapped the flood (%d of %d)", during, owned)
}
