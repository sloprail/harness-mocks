package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/subagents"
)

// claudeWorktreeLayout is where Claude Code puts an isolated sub-agent's
// worktree and what it names its branch.
//
// sr:provides subagent-worktree-isolation/claude
var claudeWorktreeLayout = subagents.WorktreeLayout{Dir: ".claude/worktrees", Prefix: "agent-", BranchPrefix: "worktree-agent-"}

// prepareSubagent validates an Agent call and sets its sub-agent up — its id,
// its isolated worktree when asked for, its seeded sidechain transcript and
// .meta.json — without running it. A nil run comes with the tool_result the
// call is refused with.
func prepareSubagent(ctx context.Context, cfg Config, inv *hooks.Invoker, toolUseID string, rawInput json.RawMessage, tr *transcript, background bool) (*subagentRun, agentToolInput, toolexec.Result) {
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

	// At the spawn limit a sub-agent has no Agent tool to call.
	if !(subagents.Parent{ID: cfg.AgentID, Depth: cfg.spawnDepth}).CanDispatch(cfg.SpawnLimit) {
		return nil, in, atSpawnLimit(cfg)
	}

	if refusal, refused := concurrentLimitRefusal(cfg); refused {
		return nil, in, refusal
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
	// at <parent-cwd>/.claude/worktrees/agent-<agentID> on the branch
	// worktree-agent-<agentID>, so the subagent's cwd (and hence its
	// os.Getwd()-derived session coordinates) DIFFERS from the parent while the
	// session_id stays SHARED (verified against real claude). Where it goes and how
	// it falls back (a plain directory, then the parent's) is the sub-agent core's
	// (subagents.Isolate); a REAL directory is bound, and it is where both the
	// sub-agent's hooks and its tool calls run. Any other isolation value shares
	// the parent cwd.
	subCwd := cfg.Cwd
	branch := ""
	var cleanup func(context.Context) bool
	hookMade := false
	if in.Isolation == "worktree" {
		path, hooked, err := hookedWorktree(ctx, cfg, inv, "agent-"+agentID)
		if hooked && err != nil {
			return nil, in, toolexec.Result{Output: "Error: could not create the worktree: " + err.Error(), IsError: true}
		}
		if hooked {
			subCwd, hookMade = path, true
		} else {
			iso := subagents.Isolate(cfg.Cwd, agentID, claudeWorktreeLayout, subagents.BindGit(ctx, cfg.Cwd))
			subCwd = iso.Cwd
			cleanup = iso.Cleanup
			if iso.Worktree != nil {
				branch = iso.Worktree.Branch
			}
			for _, note := range iso.Notes {
				fmt.Fprintf(cfg.Stderr, "claude-mock: isolation=worktree: %s\n", note)
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
	sidechain := claudeSubagentLayout.Path(sessionFile, agentID)
	place := subagents.Place(subagents.Parent{ID: cfg.AgentID, Depth: cfg.spawnDepth})
	shape := "foreground"
	if background {
		shape = "background"
	}
	meta := subagentMeta{
		AgentType: agentType, Description: in.Description, ToolUseID: toolUseID,
		ParentAgentID: place.ParentID, SpawnDepth: place.Depth,
		RequestShape: shape, RequestNonInteractive: true, Model: in.Model,
	}
	if agentType == forkAgentType {
		meta.IsFork = true
		if meta.Model == "" {
			meta.Model = "inherit"
		}
	}
	if subCwd != cfg.Cwd {
		meta.WorktreePath, meta.WorktreeBranch = subCwd, branch
		meta.SpawnedWithWorktree = branch != "" || hookMade
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

	cfg.bg.stats.Spawn(askOf(rawInput), background, meta.SpawnDepth, cfg.AgentID != "", agentType)
	script := resolveSubagentScript(in.Script)
	if script != "" { // the position a script's gate names it by
		cfg.steps.spawn(agentID, !background)
	}
	return &subagentRun{
		parent: cfg, subCwd: subCwd, agentID: agentID, agentType: agentType,
		sidechain: sidechain, parentReported: tr.reported, sessionFile: sessionFile, spawnDepth: meta.SpawnDepth,
		toolUseID: toolUseID, description: in.Description, outputFile: outFile,
		script: script, prompt: in.Prompt, background: background, cleanup: cleanup, branch: branch, limit: definitionTurnLimit(cfg, in.SubagentType),
	}, in, toolexec.Result{}
}
