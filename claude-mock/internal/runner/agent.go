package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"time"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
)

// The Agent tool spawns a nested subagent. Real Claude renamed the Task tool to
// Agent in v2.1.63 but kept Task as an accepted alias, so the mock honours both.
// sr:docs https://code.claude.com/docs/en/sub-agents
const (
	toolNameAgent = "Agent"
	toolNameTask  = "Task"
)

// envSubagentScript names the fallback subagent scenario script used when an
// Agent tool_use does not carry an explicit `script` field.
const envSubagentScript = "A10N_MOCK_SUBAGENT_SCRIPT"

// envStopHookBlockCap is the REAL Claude Code environment variable that controls
// how many CONSECUTIVE times a Stop/SubagentStop hook may block the turn from
// ending before Claude Code overrides it. Default is 8; 0 disables the cap
// (unlimited); raise it when a hook needs more iterations to resolve. The mock
// honours the same variable + default so it imitates production exactly. This is
// DISTINCT from the task's metadata.max_retries (the executor's verify budget):
// both exist in parallel — max_retries bounds verify attempts (the hook stops
// blocking once it's spent), this caps the hook-block loop itself as a backstop.
// sr:docs https://code.claude.com/docs/en/env-vars
// sr:docs https://code.claude.com/docs/en/hooks#subagentstop
const envStopHookBlockCap = "CLAUDE_CODE_STOP_HOOK_BLOCK_CAP"

const defaultStopHookBlockCap = 8

// stopHookBlockCap reads CLAUDE_CODE_STOP_HOOK_BLOCK_CAP (default 8). A returned
// 0 means "no cap" (the env was set to 0): callers treat 0 as unlimited.
func stopHookBlockCap() int {
	if v := os.Getenv(envStopHookBlockCap); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return defaultStopHookBlockCap
}

// isAgentTool reports whether toolName is the Agent tool or its Task alias.
func isAgentTool(toolName string) bool {
	return toolName == toolNameAgent || toolName == toolNameTask
}

// agentToolInput is the subset of the Agent (alias Task) tool_use input the mock
// understands. The real claude binary emits {description, prompt, subagent_type,
// isolation}; the mock additionally accepts a `script` field naming the subagent's
// scenario script (real claude lets the LLM decide subagent behaviour — the mock
// cannot, so the test supplies a script, mirroring the top-level --script).
// sr:docs https://code.claude.com/docs/en/sub-agents
type agentToolInput struct {
	Description  string `json:"description,omitempty"`
	Prompt       string `json:"prompt,omitempty"`
	SubagentType string `json:"subagent_type,omitempty"`
	// Isolation="worktree" makes the mock REPORT the subagent's isolated worktree cwd
	// (<parent>/.claude/worktrees/agent-<id>) in the SubagentStart/Stop payloads and the
	// seeded transcript — matching real claude, where the subagent's os.Getwd() differs
	// from the parent while the session_id stays shared. The mock does not physically
	// bind a git worktree (its internal machinery stays on the parent cwd); it only
	// mirrors the OBSERVED identity. Any other isolation value shares the parent cwd.
	Isolation string `json:"isolation,omitempty"`
	// Model is the optional model override the real Agent input accepts; the
	// mock reports it as an async receipt's resolvedModel.
	Model string `json:"model,omitempty"`
	// Script is the absolute path to the subagent's scenario script (mock-only).
	Script string `json:"script,omitempty"`
}

// runAgentTool runs a foreground Agent (alias Task) call, the way the real
// claude binary handles one:
//  1. Validate the input: description and prompt are required strings (the
//     2.1.282 Agent input schema); a call without them is refused with an
//     InputValidationError tool_result and nothing runs.
//  2. Mint the agent id and seed the sub-agent's own transcript
//     (<session>/subagents/agent-<id>.jsonl) with the dispatch prompt.
//  3. Fire SubagentStart. It cannot block: an exit 2 is recorded in the
//     sub-agent's own transcript as a non-blocking error and the sub-agent
//     runs anyway (docs, "Exit code 2 behavior per event"; a controlled
//     2.1.282 run).
//  4. Run the sub-agent, then fire SubagentStop. A SubagentStop block re-runs
//     the sub-agent with the feedback, as real Claude Code keeps a sub-agent
//     running, up to CLAUDE_CODE_STOP_HOOK_BLOCK_CAP. What the hook said is
//     recorded in the sub-agent's own file.
//  5. Return the tool_result real Claude Code returns for a finished
//     foreground sub-agent (buildAgentResult).
//
// toolUseID is the id of the Agent/Task tool_use that spawned the sub-agent;
// real Claude Code records it as the toolUseId of the sub-agent's .meta.json.
//
// sr:docs https://code.claude.com/docs/en/sub-agents
// sr:docs https://code.claude.com/docs/en/hooks#subagentstart
func runAgentTool(ctx context.Context, cfg Config, inv *hooks.Invoker, toolUseID string, rawInput json.RawMessage, tr *transcript) toolexec.Result {
	sub, _, errRes := prepareSubagent(ctx, cfg, inv, toolUseID, rawInput, tr, false)
	if sub == nil {
		return errRes
	}
	started := time.Now()
	out := sub.execute(ctx, inv, cfg.bg, sub.prompt)
	cfg.bg.stats.End(out.failure != "")
	var in agentToolInput
	_ = json.Unmarshal(rawInput, &in)
	worktree := ""
	if sub.subCwd != cfg.Cwd && !sub.cleanupWorktree(ctx) {
		worktree = sub.subCwd
	}
	return buildAgentResult(sub, in, cfg.Model, out, time.Since(started).Milliseconds(), worktree)
}

// resolveSubagentScript picks the subagent scenario script: the Agent input's
// `script` field wins, then the A10N_MOCK_SUBAGENT_SCRIPT env var, else "".
// sr:invariant subagent-script
func resolveSubagentScript(fromInput string) string {
	if fromInput != "" {
		return fromInput
	}
	return os.Getenv(envSubagentScript)
}

// newAgentID returns an agent id in the shape real Claude Code mints: "a"
// followed by 16 hex digits (every agentId in the real transcripts, e.g.
// a1e3c03d8ae9ab06a).
func newAgentID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "a" + hex.EncodeToString(b[:]), nil
}
