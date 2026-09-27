package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func finishedTask(t *backgroundTask) *backgroundTask {
	t.done = make(chan struct{})
	close(t.done)
	return t
}

func TestNotification_Bash(t *testing.T) {
	ok := finishedTask(&backgroundTask{id: "b1", toolUseID: "toolu_1", description: "build", outputFile: "/o"})
	assert.Equal(t, "<task-notification>\n<task-id>b1</task-id>\n<tool-use-id>toolu_1</tool-use-id>\n<output-file>/o</output-file>\n<status>completed</status>\n<summary>Background command \"build\" completed (exit code 0)</summary>\n</task-notification>", ok.notification())
	bad := finishedTask(&backgroundTask{id: "b2", toolUseID: "toolu_2", description: "test", outputFile: "/o", exitCode: 144})
	assert.Equal(t, "failed", bad.status())
	assert.Equal(t, `Background command "test" failed with exit code 144`, bad.summary())
	stopped := finishedTask(&backgroundTask{id: "b3", description: "long", outputFile: "/o"})
	stopped.killed.Store(true)
	assert.Equal(t, "stopped", stopped.status())
	var frame map[string]any
	require.NoError(t, json.Unmarshal(stopped.streamFrame("s"), &frame))
	assert.Equal(t, "task_notification", frame["subtype"])
	assert.Equal(t, "stopped", frame["status"])
	assert.Equal(t, "long", frame["summary"])
}

func TestNotification_Agent(t *testing.T) {
	ok := finishedTask(&backgroundTask{id: "a1", toolUseID: "toolu_1", agent: true, description: "research", outputFile: "/o", result: "R", toolUses: 3, durationMs: 42})
	assert.Equal(t, "<task-notification>\n<task-id>a1</task-id>\n<tool-use-id>toolu_1</tool-use-id>\n<output-file>/o</output-file>\n<status>completed</status>\n<summary>Agent \"research\" finished</summary>\n<note>"+agentNotificationNote+"</note>\n<result>R</result>\n<usage><subagent_tokens>0</subagent_tokens><tool_uses>3</tool_uses><duration_ms>42</duration_ms></usage>\n</task-notification>", ok.notification())
	failed := finishedTask(&backgroundTask{id: "a2", agent: true, description: "research", outputFile: "/o", failure: "boom"})
	n := failed.notification()
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

func TestLaunchBash_ReceiptAndCompletion(t *testing.T) {
	cfg := testCfg(t)
	b := newBackgroundTasks()
	defer b.shutdown()
	res := b.launchBash(cfg, "toolu_1", json.RawMessage(`{"command":"echo hi","run_in_background":true}`))
	require.False(t, res.IsError)
	tur := res.ToolUseResult.(map[string]any)
	id := tur["backgroundTaskId"].(string)
	out := filepath.Join(tasksDir(cfg.Cwd, "sid"), id+".output")
	assert.Equal(t, "Command running in background with ID: "+id+". Output is being written to: "+out+". You will be notified when it completes. To check interim output, use Read on that file path.", res.Output)
	assert.NotContains(t, tur, "backgroundEndsWithFinalResponse")
	task := b.awaitAfterTurn(context.Background(), "")
	assert.Nil(t, task, "a -p session does not wait for a background command")
	require.Eventually(t, func() bool { return len(b.takeFinished("")) == 1 }, 5*time.Second, 20*time.Millisecond)
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "hi\n\n[exited with code 0]\n", string(data))
	assert.Empty(t, b.takeFinished(""), "a task is handed over once")
}

func TestLaunchBash_InsideAForegroundSubagent(t *testing.T) {
	cfg := testCfg(t)
	cfg.SyncSubagent, cfg.AgentID = true, "a1"
	b := newBackgroundTasks()
	defer b.shutdown()
	res := b.launchBash(cfg, "toolu_1", json.RawMessage(`{"command":"sleep 30","run_in_background":true}`))
	assert.Contains(t, res.Output, "it is terminated when you give your final response")
	assert.Equal(t, true, res.ToolUseResult.(map[string]any)["backgroundEndsWithFinalResponse"])
	assert.Len(t, b.running(), 1)
	b.stopOwned(Config{AgentID: "other", Out: &bytes.Buffer{}})
	assert.Len(t, b.running(), 1, "only the owner's commands end with its final response")
	started := time.Now()
	b.stopOwned(cfg)
	assert.Less(t, time.Since(started), 5*time.Second)
	assert.Empty(t, b.running())
	assert.Contains(t, cfg.Out.(*bytes.Buffer).String(), `"status":"stopped"`)
	assert.Empty(t, b.takeFinished("a1"), "a stopped command is not handed over")
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
	b.shutdown()
	assert.Less(t, time.Since(started), 5*time.Second, "shutdown waits for the group, which is gone")
	time.Sleep(2500 * time.Millisecond)
	_, err := os.Stat(marker)
	assert.True(t, os.IsNotExist(err), "the command's child was killed with it")
}

func TestAwaitAfterTurn_WaitsForAgentsInLaunchOrder(t *testing.T) {
	b := newBackgroundTasks()
	defer b.shutdown()
	a := &backgroundTask{id: "a", agent: true, done: make(chan struct{})}
	c := &backgroundTask{id: "c", done: make(chan struct{})}
	b.add(a)
	b.add(c)
	go func() {
		time.Sleep(100 * time.Millisecond)
		b.finish(c)
		time.Sleep(100 * time.Millisecond)
		b.finish(a)
	}()
	first := b.awaitAfterTurn(context.Background(), "")
	require.NotNil(t, first)
	assert.Equal(t, "c", first.id, "whatever finishes first while an agent runs")
	second := b.awaitAfterTurn(context.Background(), "")
	require.NotNil(t, second)
	assert.Equal(t, "a", second.id)
	assert.Nil(t, b.awaitAfterTurn(context.Background(), ""))
}

func TestRunning_ListsTheSessionsTasks(t *testing.T) {
	b := newBackgroundTasks()
	b.add(&backgroundTask{id: "b1", description: "d", command: "c", done: make(chan struct{})})
	b.add(&backgroundTask{id: "a1", agent: true, owner: "x", description: "d", agentType: "t", done: make(chan struct{})})
	b.add(finishedTask(&backgroundTask{id: "b2"}))
	got, _ := json.Marshal(b.running())
	assert.JSONEq(t, `[{"id":"b1","type":"shell","status":"running","description":"d","command":"c"},{"id":"a1","type":"subagent","status":"running","description":"d","agent_type":"t"}]`, string(got))
	var nilB *backgroundTasks
	assert.Equal(t, 0, len(nilB.running()))
}
