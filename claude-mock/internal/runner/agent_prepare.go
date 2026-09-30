package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// prepareSubagent validates an Agent call and sets its sub-agent up — its id,
// its isolated worktree when asked for, its seeded sidechain transcript and
// .meta.json — without running it. A nil run comes with the tool_result the
// call is refused with.
func prepareSubagent(cfg Config, toolUseID string, rawInput json.RawMessage, tr *transcript, background bool) (*subagentRun, agentToolInput, toolexec.Result) {
	var in agentToolInput
	var probe map[string]any
	if err := json.Unmarshal(rawInput, &probe); err != nil {
		return nil, in, toolexec.Result{Output: "<tool_use_error>InputValidationError: Agent was called with input that could not be parsed as JSON.</tool_use_error>", IsError: true}
	}
	_ = json.Unmarshal(rawInput, &in)
	var missing []string
	for _, field := range []string{"description", "prompt"} {
		if _, ok := probe[field].(string); !ok {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		return nil, in, inputValidationError("Agent", missing)
	}

	agentType := in.SubagentType
	if agentType == "" {
		agentType = "general-purpose"
	}
	agentID, err := newAgentID()
	if err != nil {
		return nil, in, toolexec.Result{Output: fmt.Sprintf("Agent: generate agent_id: %v", err), IsError: true}
	}

	// isolation="worktree" — the real claude runs the subagent in a fresh git worktree
	// at <parent-cwd>/.claude/worktrees/agent-<agentID>, so the subagent's cwd (and
	// hence its os.Getwd()-derived session coordinates) DIFFERS from the parent while
	// the session_id stays SHARED (verified against real claude). The mock binds a REAL
	// directory here — a `git worktree add` when cfg.Cwd is a git repo, a plain mkdir
	// otherwise — and it is where both the sub-agent's hooks and its tool calls run.
	// Any other isolation value shares the parent cwd.
	// sr:docs https://code.claude.com/docs/en/sub-agents
	subCwd := cfg.Cwd
	branch := ""
	if in.Isolation == "worktree" {
		subCwd = filepath.Join(cfg.Cwd, ".claude", "worktrees", "agent-"+agentID)
		branch = "worktree-agent-" + agentID
		if err := bindWorktree(context.Background(), cfg.Cwd, subCwd, branch); err != nil {
			branch = ""
			fmt.Fprintf(cfg.Stderr, "claude-mock: isolation=worktree: bind %s: %v (falling back to a plain directory)\n", subCwd, err)
			if mkErr := os.MkdirAll(subCwd, 0o755); mkErr != nil {
				fmt.Fprintf(cfg.Stderr, "claude-mock: isolation=worktree: mkdir %s: %v — isolation NOT applied, sharing parent cwd\n", subCwd, mkErr)
				subCwd = cfg.Cwd
			}
		}
	}

	// The sub-agent's own transcript sits beside the session's:
	// <session file without .jsonl>/subagents/agent-<id>.jsonl — for a nested
	// sub-agent too, since real Claude Code keeps every sub-agent of a session
	// in the one subagents/ directory.
	sessionFile := cfg.sessionFile
	if sessionFile == "" {
		sessionFile = tr.path
	}
	sidechain := filepath.Join(strings.TrimSuffix(sessionFile, ".jsonl"), "subagents", "agent-"+agentID+".jsonl")
	shape := "foreground"
	if background {
		shape = "background"
	}
	meta := subagentMeta{
		AgentType: agentType, Description: in.Description, ToolUseID: toolUseID,
		ParentAgentID: cfg.AgentID, SpawnDepth: cfg.spawnDepth + 1,
		RequestShape: shape, RequestNonInteractive: true, Model: in.Model,
	}
	if subCwd != cfg.Cwd {
		meta.WorktreePath, meta.WorktreeBranch = subCwd, branch
		meta.SpawnedWithWorktree = branch != ""
	}
	seedSubagentTranscript(sidechain, subCwd, cfg.SessionID, agentID, in.Prompt, meta)

	// Every sub-agent, foreground or background, has tasks/<agentId>.output,
	// a symlink to its transcript (the stream's task frames name it for
	// foreground sub-agents too: F:meta, F:hookerrors).
	taskDir := tasksDir(cfg.Cwd, cfg.SessionID)
	_ = os.MkdirAll(taskDir, 0o755)
	outFile := filepath.Join(taskDir, agentID+".output")
	_ = os.Remove(outFile)
	_ = os.Symlink(sidechain, outFile)

	return &subagentRun{
		parent: cfg, subCwd: subCwd, agentID: agentID, agentType: agentType,
		sidechain: sidechain, parentReported: tr.reported, sessionFile: sessionFile, spawnDepth: meta.SpawnDepth,
		toolUseID: toolUseID, description: in.Description, outputFile: outFile,
		script: resolveSubagentScript(in.Script), prompt: in.Prompt, background: background,
	}, in, toolexec.Result{}
}

// bindWorktree makes worktreeDir a REAL, usable directory for isolation="worktree": a genuine
// `git worktree add` of parentCwd's current HEAD when parentCwd is a git repo with at least one
// commit (mirroring real claude's own mechanism — a real worktree, same repo, isolated files),
// or a plain empty directory otherwise (parentCwd isn't a git repo, or has no commits yet — a
// worktree needs a HEAD to branch from). The caller (runAgentTool) already falls back to a plain
// mkdir on any error this returns, so this only needs to try the real thing and report failure.
func bindWorktree(ctx context.Context, parentCwd, worktreeDir, branch string) error {
	if err := os.MkdirAll(filepath.Dir(worktreeDir), 0o755); err != nil {
		return fmt.Errorf("mkdir parent: %w", err)
	}
	checkGit := exec.CommandContext(ctx, "git", "-C", parentCwd, "rev-parse", "--is-inside-work-tree")
	if err := checkGit.Run(); err != nil {
		return fmt.Errorf("not a git repo: %w", err)
	}
	checkHead := exec.CommandContext(ctx, "git", "-C", parentCwd, "rev-parse", "--verify", "HEAD")
	if err := checkHead.Run(); err != nil {
		return fmt.Errorf("no HEAD (no commits yet): %w", err)
	}
	// A worktree on a new branch worktree-agent-<id> at the current HEAD — the branch real
	// Claude Code creates (every real isolated sub-agent's .meta.json names it).
	add := exec.CommandContext(ctx, "git", "-C", parentCwd, "worktree", "add", "-b", branch, worktreeDir, "HEAD")
	var stderr bytes.Buffer
	add.Stderr = &stderr
	if err := add.Run(); err != nil {
		return fmt.Errorf("git worktree add: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
