package runner

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/a10n-build/a10n-cli/services/claude-mock/internal/hooks"
	"github.com/a10n-build/a10n-cli/services/claude-mock/internal/toolexec"
)

// The Agent tool spawns a nested subagent. Real Claude renamed the Task tool to
// Agent in v2.1.63 but kept Task as an accepted alias, so the mock honours both.
// a10n:docs https://code.claude.com/docs/en/sub-agents
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
// a10n:docs https://code.claude.com/docs/en/env-vars
// a10n:docs https://code.claude.com/docs/en/hooks#subagentstop
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
// a10n:docs https://code.claude.com/docs/en/sub-agents
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
	// Script is the absolute path to the subagent's scenario script (mock-only).
	Script string `json:"script,omitempty"`
}

// runAgentTool spawns a nested subagent run, mirroring how the real claude binary
// handles an Agent tool_use:
//  1. Generate a unique per-subagent agent_id.
//  2. Fire SubagentStart with {agent_type, agent_id}. SubagentStart cannot block.
//  3. Run the subagent (its own conversation, sharing the parent session_id).
//  4. Fire SubagentStop with {agent_type, agent_id, stop_reason}. SubagentStop CAN
//     block (exit 2 / decision:block) — on a block the subagent re-runs once more,
//     then gives up. A block is never silently ignored.
//  5. Return a tool_result whose content carries the literal `agentId: <id>` plus
//     the subagent's final result text.
//
// a10n:docs https://code.claude.com/docs/en/sub-agents
// a10n:docs https://code.claude.com/docs/en/hooks#subagentstart
func runAgentTool(ctx context.Context, cfg Config, inv *hooks.Invoker, rawInput json.RawMessage) toolexec.Result {
	var in agentToolInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return toolexec.Result{Output: fmt.Sprintf("Agent: invalid tool input: %v", err), IsError: true}
	}

	agentType := in.SubagentType
	if agentType == "" {
		agentType = "general-purpose"
	}

	agentID, err := newAgentID()
	if err != nil {
		return toolexec.Result{Output: fmt.Sprintf("Agent: generate agent_id: %v", err), IsError: true}
	}

	// isolation="worktree" — the real claude runs the subagent in a fresh git worktree
	// at <parent-cwd>/.claude/worktrees/agent-<agentID>, so the subagent's cwd (and
	// hence its os.Getwd()-derived session coordinates) DIFFERS from the parent while
	// the session_id stays SHARED (verified against real claude). The mock does not bind
	// a real worktree, but it MUST report that cwd everywhere the subagent's env is
	// observed — SubagentStart/Stop payloads, the seeded transcript's `cwd`, and the
	// nested run — so a hook (and any a10n tool the subagent shells out to) sees the
	// isolated cwd, not the parent's. Any other isolation value shares the parent cwd.
	// a10n:docs https://code.claude.com/docs/en/sub-agents
	subCwd := cfg.Cwd
	if in.Isolation == "worktree" {
		subCwd = filepath.Join(cfg.Cwd, ".claude", "worktrees", "agent-"+agentID)
	}

	// The subagent gets its OWN sidechain transcript under the parent session's
	// subagents/ dir — mirroring the real claude layout, whose subagent transcript's
	// first record IS the dispatch prompt. Seed it before SubagentStart so a hook can
	// read the prompt (e.g. an embedded `--task-id`) from transcript_path. configDir
	// is resolved through CLAUDE_CONFIG_DIR so tests stay isolated. The transcript's
	// project dir keys off the PARENT cwd (real claude nests subagents/ under the
	// parent session's transcript dir); only the recorded `cwd` field is the subagent's.
	// a10n:docs https://code.claude.com/docs/en/agent-sdk/sessions (CLAUDE_CONFIG_DIR + projects/<encoded-cwd>)
	configDir := resolveConfigDir(cfg.ConfigDir)
	transcriptPath := seedSubagentTranscript(configDir, cfg.Cwd, subCwd, cfg.SessionID, agentID, agentType, in.Prompt)

	// SubagentStart — cannot block; a blocking error here is treated as a hard
	// failure of the Agent tool (the real claude never proceeds past a refused start).
	// a10n:docs https://code.claude.com/docs/en/hooks#subagentstart
	if _, err := inv.Fire(ctx, hooks.Input{
		SessionID:      cfg.SessionID,
		Cwd:            subCwd,
		TranscriptPath: transcriptPath,
		HookEventName:  hooks.EventSubagentStart,
		AgentType:      agentType,
		AgentID:        agentID,
	}); err != nil {
		return toolexec.Result{Output: fmt.Sprintf("Agent: SubagentStart blocked: %v", err), IsError: true}
	}

	scriptPath := resolveSubagentScript(in.Script)

	// Run the subagent, then fire SubagentStop. SubagentStop is the verify/handoff
	// seam: when it returns {"decision":"block"} (a failed verify with budget
	// remaining) real Claude RE-RUNS THE SAME TURN with the feedback — it does NOT
	// hand control back to the orchestrator. We reproduce that contract here: loop
	// run-subagent → fire-SubagentStop until the hook stops blocking (clean stop =
	// verify passed, OR the executor parked the task itself e.g. retries_exhausted,
	// at which point the hook allows the stop). stop_hook_active is set on every
	// re-fire after the first (the real Claude flag a hook checks to break its own
	// recursion). maxStopBlocks is the mock runaway guard (default 8) — the
	// stand-in for Claude's context limit, NOT the task's max_retries budget.
	// a10n:docs https://code.claude.com/docs/en/hooks#subagentstop
	blockCap := stopHookBlockCap() // 0 = unlimited
	finalText := runSubagent(ctx, cfg, agentID, scriptPath, in.Prompt)
	for turn := 0; ; turn++ {
		blocked, reason := fireSubagentStop(ctx, cfg, subCwd, inv, agentType, agentID, transcriptPath, turn > 0)
		if !blocked {
			break // clean stop — verify passed or the task was parked by the executor
		}
		if blockCap > 0 && turn >= blockCap {
			fmt.Fprintf(cfg.Stderr, "claude-mock: SubagentStop still blocked after %d turns (cap) — giving up\n", blockCap)
			break
		}
		fmt.Fprintf(cfg.Stderr, "claude-mock: SubagentStop blocked (%s) — re-running subagent (turn %d)\n", reason, turn+1)
		finalText = runSubagent(ctx, cfg, agentID, scriptPath, in.Prompt)
	}

	return toolexec.Result{Output: buildAgentResultContent(agentID, agentType, finalText)}
}

// resolveSubagentScript picks the subagent scenario script: the Agent input's
// `script` field wins, then the A10N_MOCK_SUBAGENT_SCRIPT env var, else "".
func resolveSubagentScript(fromInput string) string {
	if fromInput != "" {
		return fromInput
	}
	return os.Getenv(envSubagentScript)
}

// runSubagent executes the subagent scenario and returns its final result text.
//
// The nested run shares the parent session_id and cwd, runs with IsResume=true
// (so the test's own SubagentStart/Stop wiring applies inside it if desired) but
// with SuppressSubagentHooks=true so it does NOT double-fire SubagentStart/Stop —
// the Agent-tool layer owns those, fired WITH the agent_id. Its JSONL is captured
// (not forwarded to the parent's stdout): in real claude the subagent runs as a
// sidechain and only the Agent tool_result surfaces to the parent stream.
//
// If no script is configured, the subagent is a graceful no-op (not an error).
//
// a10n:docs https://code.claude.com/docs/en/sub-agents
func runSubagent(ctx context.Context, cfg Config, agentID, scriptPath, prompt string) string {
	if scriptPath == "" {
		return "no subagent script"
	}

	var buf bytes.Buffer
	subCfg := Config{
		ScriptPath:            scriptPath,
		SessionID:             cfg.SessionID,
		AgentID:               agentID,
		IsResume:              true,
		Prompt:                prompt,
		Cwd:                   cfg.Cwd,
		ProjectDir:            cfg.ProjectDir,
		ConfigDir:             cfg.ConfigDir,
		PluginCacheDir:        cfg.PluginCacheDir,
		Stderr:                cfg.Stderr,
		Out:                   &buf,
		SuppressSubagentHooks: true,
	}
	if err := Run(ctx, subCfg); err != nil {
		fmt.Fprintf(cfg.Stderr, "claude-mock: subagent run error: %v\n", err)
	}
	return lastResultText(buf.Bytes())
}

// fireSubagentStop fires SubagentStop with the agent_id (+ the subagent's
// transcript_path, as the real hook payload carries) and returns the (possibly
// blocking) error from the hook so the caller can react to a block.
// a10n:docs https://code.claude.com/docs/en/hooks#subagentstop
// fireSubagentStop fires the SubagentStop hook and reports whether it BLOCKED.
// A block is signalled two ways, both meaning "re-run the turn": the hook exits
// 2 (returned as a non-nil error from Fire), OR it exits 0 with a
// {"decision":"block"} stdout frame (the SubagentStop contract real Claude
// honours — exit-0 + decision, not a process error). The caller loops on a block.
// subCwd is the subagent's OWN cwd (its isolated worktree under isolation="worktree",
// else == cfg.Cwd) — reported in the SubagentStop payload's `cwd` field so a hook sees
// the isolated cwd, matching real claude. The mock's internal machinery (session file,
// block loop, hook invoker) stays on cfg since the mock does not bind a real worktree.
func fireSubagentStop(ctx context.Context, cfg Config, subCwd string, inv *hooks.Invoker, agentType, agentID, transcriptPath string, stopHookActive bool) (blocked bool, reason string) {
	out, err := inv.Fire(ctx, hooks.Input{
		SessionID:           cfg.SessionID,
		Cwd:                 subCwd,
		TranscriptPath:      transcriptPath,
		AgentTranscriptPath: transcriptPath,
		HookEventName:       hooks.EventSubagentStop,
		StopReason:          "end_turn",
		AgentType:           agentType,
		AgentID:             agentID,
		StopHookActive:      stopHookActive,
	})
	if err != nil {
		return true, err.Error()
	}
	if out.Decision == "block" {
		return true, out.Reason
	}
	return false, ""
}

// lastResultText scans captured JSONL for the last result frame and returns its
// `result` text. Returns "" when no result frame is present.
func lastResultText(out []byte) string {
	text := ""
	for _, line := range bytes.Split(out, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var rec struct {
			Type   string `json:"type"`
			Result string `json:"result"`
		}
		if json.Unmarshal(line, &rec) != nil {
			continue
		}
		if rec.Type == "result" {
			text = rec.Result
		}
	}
	return text
}

// buildAgentResultContent renders the Agent tool_result content string. It MUST
// include the literal `agentId: <id>` — tests and future resume rely on parsing
// it back out — followed by the subagent's final result text.
// a10n:docs https://code.claude.com/docs/en/sub-agents
func buildAgentResultContent(agentID, agentType, finalText string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "agentId: %s\n", agentID)
	fmt.Fprintf(&b, "agentType: %s\n", agentType)
	b.WriteString(finalText)
	return b.String()
}

// newAgentID returns a short random hex id (8 bytes). crypto/rand is fine here:
// this is the mock binary, not a workflow script.
func newAgentID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
