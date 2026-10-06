package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sloprail/harness-mocks/internal/tasks"
)

// finishedTask is task t, registered and finished.
func finishedTask(t *tasks.Task) *tasks.Task {
	r := tasks.NewRegistry()
	r.Add(t)
	r.Finish(t)
	return t
}

// newTask is a task of kind with its fields set by set.
func newTask(kind tasks.Kind, id string, set func(*tasks.Task)) *tasks.Task {
	t := tasks.NewTask(kind, id)
	set(t)
	return t
}

func TestNotification_Bash(t *testing.T) {
	ok := finishedTask(newTask(tasks.Command, "b1", func(t *tasks.Task) { t.ToolUseID, t.Description, t.OutputFile = "toolu_1", "build", "/o" }))
	assert.Equal(t, "<task-notification>\n<task-id>b1</task-id>\n<tool-use-id>toolu_1</tool-use-id>\n<output-file>/o</output-file>\n<status>completed</status>\n<summary>Background command \"build\" completed (exit code 0)</summary>\n</task-notification>", taskNotification(ok))
	bad := finishedTask(newTask(tasks.Command, "b2", func(t *tasks.Task) {
		t.ToolUseID, t.Description, t.OutputFile, t.ExitCode = "toolu_2", "test", "/o", 144
	}))
	assert.Equal(t, tasks.Failed, bad.Status())
	assert.Equal(t, `Background command "test" failed with exit code 144`, taskSummary(bad))
}

// TestTaskFrames_AKilledCommandIsKilledThenStopped: a command ended at the end of a
// session streams task_started, task_updated "killed", then a task_notification
// "stopped" whose summary is its description (recorded: snapshots/runs/bgbash).
func TestTaskFrames_AKilledCommandIsKilledThenStopped(t *testing.T) {
	cfg := testCfg(t)
	b := newBackgroundTasks()
	defer b.Shutdown()
	b.launchBash(cfg, "toolu_1", json.RawMessage(`{"command":"sleep 30","description":"long","run_in_background":true}`))
	b.ReapAtExit("", 0)
	var kinds, statuses []string
	var changed []any
	var last map[string]any
	for _, l := range strings.Split(strings.TrimSpace(cfg.Out.(*bytes.Buffer).String()), "\n") {
		var f map[string]any
		require.NoError(t, json.Unmarshal([]byte(l), &f))
		kinds = append(kinds, f["subtype"].(string))
		switch f["subtype"] {
		case "task_updated":
			statuses = append(statuses, f["patch"].(map[string]any)["status"].(string))
		case "background_tasks_changed":
			changed = append(changed, f["tasks"])
		}
		last = f
	}
	assert.Equal(t, []string{"background_tasks_changed", "task_started", "background_tasks_changed", "task_updated", "task_notification"}, kinds)
	require.Len(t, changed, 2)
	running := changed[0].([]any)
	require.Len(t, running, 1, "the command is running when it starts")
	assert.Equal(t, "local_bash", running[0].(map[string]any)["task_type"])
	assert.Equal(t, "long", running[0].(map[string]any)["description"])
	assert.Equal(t, []any{}, changed[1], "none once it has ended")
	assert.Equal(t, []string{"killed"}, statuses)
	assert.Equal(t, "stopped", last["status"])
	assert.Equal(t, "long", last["summary"])
	assert.Equal(t, "sid", last["session_id"])
}

func TestNotification_Agent(t *testing.T) {
	ok := finishedTask(newTask(tasks.Agent, "a1", func(t *tasks.Task) {
		t.ToolUseID, t.Description, t.OutputFile, t.Result, t.ToolUses, t.DurationMs = "toolu_1", "research", "/o", "R", 3, 42
	}))
	assert.Equal(t, "<task-notification>\n<task-id>a1</task-id>\n<tool-use-id>toolu_1</tool-use-id>\n<output-file>/o</output-file>\n<status>completed</status>\n<summary>Agent \"research\" finished</summary>\n<note>"+agentNotificationNote+"</note>\n<result>R</result>\n<usage><subagent_tokens>0</subagent_tokens><tool_uses>3</tool_uses><duration_ms>42</duration_ms></usage>\n</task-notification>", taskNotification(ok))
	failed := finishedTask(newTask(tasks.Agent, "a2", func(t *tasks.Task) { t.Description, t.OutputFile, t.Failure = "research", "/o", "boom" }))
	n := taskNotification(failed)
	assert.Contains(t, n, "<status>failed</status>")
	assert.Contains(t, n, `<summary>Agent "research" failed: boom</summary>`)
	assert.Contains(t, n, "<note>")
	assert.NotContains(t, n, "<result>", "no result when it said nothing")
	assert.NotContains(t, n, "<usage>")
	assert.NotContains(t, n, "<tool-use-id>", "no tool-use-id when the call had none")
}

func TestChangesDirectory(t *testing.T) {
	for cmd, want := range map[string]bool{
		"cd /tmp && make": true, "make; pushd x": true, "popd": true, "chdir x | y": true,
		"make": false, "echo cd": false, "abcd x": false,
	} {
		assert.Equal(t, want, changesDirectory(cmd), cmd)
	}
}

func TestInputValidationError(t *testing.T) {
	one := inputValidationError("Agent", []string{"prompt"})
	assert.True(t, one.IsError)
	assert.Equal(t, "<tool_use_error>InputValidationError: Agent failed due to the following issue:\nThe required parameter `prompt` is missing</tool_use_error>", one.Output)
	two := inputValidationError("Agent", []string{"description", "prompt"})
	assert.Equal(t, "<tool_use_error>InputValidationError: Agent failed due to the following issues:\nThe required parameter `description` is missing\nThe required parameter `prompt` is missing</tool_use_error>", two.Output)
	assert.True(t, strings.HasPrefix(two.ToolUseResult.(string), "InputValidationError: [\n  {\n"))
}

func TestTasksDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CLAUDE_CODE_TMPDIR", tmp)
	resolved, err := filepath.EvalSymlinks(tmp)
	require.NoError(t, err)
	cwd := t.TempDir()
	encoded := nonAlphanumRe.ReplaceAllString(resolveEncodingCwd(cwd), "-")
	assert.Equal(t, filepath.Join(resolved, "claude-"+strconv.Itoa(os.Getuid()), encoded, "sid", "tasks"), tasksDir(cwd, "sid"))
}

func testCfg(t *testing.T) Config {
	t.Setenv("CLAUDE_CODE_TMPDIR", t.TempDir())
	return Config{SessionID: "sid", Cwd: t.TempDir(), Out: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
}

// sr:proves background-bash/claude
func TestLaunchBash_ReceiptAndCompletion(t *testing.T) {
	cfg := testCfg(t)
	b := newBackgroundTasks()
	defer b.Shutdown()
	res := b.launchBash(cfg, "toolu_1", json.RawMessage(`{"command":"sleep 1; echo hi","run_in_background":true}`))
	require.False(t, res.IsError)
	tur := res.ToolUseResult.(map[string]any)
	id := tur["backgroundTaskId"].(string)
	out := filepath.Join(tasksDir(cfg.Cwd, "sid"), id+".output")
	assert.Equal(t, "Command running in background with ID: "+id+". Output is being written to: "+out+". You will be notified when it completes. To check interim output, use Read on that file path.", res.Output)
	assert.NotContains(t, tur, "backgroundEndsWithFinalResponse")
	task := b.AwaitAfterTurn(context.Background(), "")
	assert.Nil(t, task, "a -p session does not wait for a background command")
	require.Eventually(t, func() bool { return len(b.TakeFinished("")) == 1 }, 30*time.Second, 20*time.Millisecond)
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "hi\n\n[exited with code 0]\n", string(data))
	assert.Empty(t, b.TakeFinished(""), "a task is handed over once")
}

// sr:proves foreground-subagent-bash-ends-with-response/claude
func TestLaunchBash_InsideAForegroundSubagent(t *testing.T) {
	cfg := testCfg(t)
	cfg.SyncSubagent, cfg.AgentID = true, "a1"
	b := newBackgroundTasks()
	defer b.Shutdown()
	res := b.launchBash(cfg, "toolu_1", json.RawMessage(`{"command":"sleep 30","run_in_background":true}`))
	assert.Contains(t, res.Output, "it is terminated when you give your final response")
	assert.Equal(t, true, res.ToolUseResult.(map[string]any)["backgroundEndsWithFinalResponse"])
	assert.Len(t, b.running(), 1)
	b.EndOfResponse("other")
	assert.Len(t, b.running(), 1, "only the owner's commands end with its final response")
	started := time.Now()
	b.EndOfResponse(cfg.AgentID)
	assert.Less(t, time.Since(started), 5*time.Second)
	assert.Empty(t, b.running())
	assert.Contains(t, cfg.Out.(*bytes.Buffer).String(), `"status":"stopped"`)
	assert.Empty(t, b.TakeFinished("a1"), "a stopped command is not handed over")
}

func TestLaunchBash_MissingCommand(t *testing.T) {
	res := newBackgroundTasks().launchBash(testCfg(t), "toolu_1", json.RawMessage(`{"run_in_background":true}`))
	assert.True(t, res.IsError)
	assert.Contains(t, res.Output, "The required parameter `command` is missing")
}

func TestShutdown_KillsTheProcessGroup(t *testing.T) {
	cfg := testCfg(t)
	marker := filepath.Join(cfg.Cwd, "child-alive")
	b := newBackgroundTasks()
	// The command's child outlives the shell unless the whole group is killed.
	b.launchBash(cfg, "toolu_1", json.RawMessage(`{"command":"(sleep 2; touch `+marker+`) & sleep 30","run_in_background":true}`))
	time.Sleep(200 * time.Millisecond)
	started := time.Now()
	b.Shutdown()
	assert.Less(t, time.Since(started), 5*time.Second, "shutdown waits for the group, which is gone")
	time.Sleep(2500 * time.Millisecond)
	_, err := os.Stat(marker)
	assert.True(t, os.IsNotExist(err), "the command's child was killed with it")
}

func TestRunning_ListsTheSessionsTasks(t *testing.T) {
	b := newBackgroundTasks()
	b.Add(newTask(tasks.Command, "b1", func(t *tasks.Task) { t.Description, t.Command = "d", "c" }))
	b.Add(newTask(tasks.Agent, "a1", func(t *tasks.Task) { t.Owner, t.Description, t.AgentType = "x", "d", "t" }))
	b.Add(finishedTask(tasks.NewTask(tasks.Command, "b2")))
	got, _ := json.Marshal(b.running())
	assert.JSONEq(t, `[{"id":"b1","type":"shell","status":"running","description":"d","command":"c"},{"id":"a1","type":"subagent","status":"running","description":"d","agent_type":"t"}]`, string(got))
	var nilB *backgroundTasks
	assert.Equal(t, 0, len(nilB.running()))
}

// TestStreamLinesStayWholeUnderConcurrentFrames: the session stream is written
// by the turn loop and, concurrently, by background sub-agents' task frames.
// Every line must reach the stream whole — a line and its newline in one
// Write — or a frame lands between them and both lines become unparseable.
func TestStreamLinesStayWholeUnderConcurrentFrames(t *testing.T) {
	var buf bytes.Buffer
	lw := &lockedWriter{w: &buf}
	cfg := Config{SessionID: "s", Out: lw, stream: lw}
	payload := strings.Repeat("x", 512)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(2)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				writeStreamLine(cfg, []byte(`{"type":"assistant","n":`+strconv.Itoa(i)+`,"pad":"`+payload+`"}`))
			}
		}(g)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				writeFrame(cfg, map[string]any{"type": "system", "subtype": "task_progress", "task_id": strconv.Itoa(g)})
			}
		}(g)
	}
	wg.Wait()
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	require.Len(t, lines, 8*2000*2)
	for i, l := range lines {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(l), &m), "line %d is not whole: %.120s", i, l)
	}
}
