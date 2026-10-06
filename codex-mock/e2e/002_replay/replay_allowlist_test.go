package e2e

// notReplaying are the recorded runs whose replay is not green yet, and why.
// The list only shrinks: an entry whose run is gone, or whose run now replays
// green, fails TestGeneratedReplay (except a "flaky:" entry, which is green in
// some runs and so is only checked for its run being there). "adapter:" is
// something of the recording the codex adapter cannot reproduce yet;
// "untriaged:" is a replay that differs and has not been looked at (the mock,
// the adapter or the recording may be wrong).
var notReplaying = map[string]string{
	"bg-bash-reaped-at-exit":           "adapter: an exec_command whose cmd is not a string literal",
	"noninteractive-run-output-schema": "adapter: the setup has args, which the adapter does not install",
	"noninteractive-run-text-output":   "adapter: recorded with another command line: \"codex exec --skip-git-repo-check --dangerously-bypass-approvals-and-sandbox --dangerously-bypass-hook-trust -m gpt-5.6-luna\"",
	"subagent-stop-block-loop-cap":     "adapter: the model called wait: the adapter maps only exec",
	"task-stream-frames":               "adapter: the model called tools.write_stdin: the adapter maps exec_command, spawn_agent and wait_agent",
}
