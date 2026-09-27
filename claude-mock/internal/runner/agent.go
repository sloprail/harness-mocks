package runner

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

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
//     then gives up. A block is never silently ignored: it re-runs the turn AND
//     lands a hook_blocking_error attachment in the transcript, the same channel
//     the root's Stop uses, so the refusal's TEXT is readable afterwards.
//  5. Return a tool_result whose content carries the literal `agentId: <id>` plus
//     the subagent's final result text.
//
// sessionFile is the parent conversation's transcript, needed for (4)'s
// attachment. The subagent's own refusal is recorded in the DISPATCHING
// session's record because that is the conversation a reader has: the subagent's
// nested run captures its JSONL to a buffer rather than to a file of its own
// (see runSubagent), exactly as a real sidechain surfaces only through its
// parent.
//
// toolUseID is the id of the Agent/Task tool_use block that spawned this
// subagent. Real Claude Code records it as the `toolUseId` in the subagent's
// .meta.json sidecar; a consumer deriving the subagent's parentPath (which
// tool_use in the PARENT transcript spawned this sidechain) reads it. Empty only
// when the spawning tool_use carried no id.
//
// sr:docs https://code.claude.com/docs/en/sub-agents
// sr:docs https://code.claude.com/docs/en/hooks#subagentstart
func runAgentTool(ctx context.Context, cfg Config, inv *hooks.Invoker, toolUseID string, rawInput json.RawMessage, tr *transcript) toolexec.Result {
	res, _ := runAgentToolFor(ctx, cfg, inv, toolUseID, rawInput, tr, "")
	return res
}

// runAgentToolFor is runAgentTool with the agent id chosen by the caller (a
// background launch hands out the id in its receipt before the agent runs) and
// the id actually used returned beside the result. An empty agentID mints one.
func runAgentToolFor(ctx context.Context, cfg Config, inv *hooks.Invoker, toolUseID string, rawInput json.RawMessage, tr *transcript, agentID string) (toolexec.Result, string) {
	var in agentToolInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return toolexec.Result{Output: fmt.Sprintf("Agent: invalid tool input: %v", err), IsError: true}, ""
	}

	agentType := in.SubagentType
	if agentType == "" {
		agentType = "general-purpose"
	}

	if agentID == "" {
		var err error
		agentID, err = newAgentID()
		if err != nil {
			return toolexec.Result{Output: fmt.Sprintf("Agent: generate agent_id: %v", err), IsError: true}, ""
		}
	}

	// isolation="worktree" — the real claude runs the subagent in a fresh git worktree
	// at <parent-cwd>/.claude/worktrees/agent-<agentID>, so the subagent's cwd (and
	// hence its os.Getwd()-derived session coordinates) DIFFERS from the parent while
	// the session_id stays SHARED (verified against real claude). The mock binds a REAL
	// directory here (not just a reported string) — a `git worktree add` when cfg.Cwd is a
	// git repo (matching real claude's own mechanism exactly: a genuine worktree, same repo,
	// isolated files), falling back to a plain mkdir when it is not (or the git command
	// fails — e.g. no commits yet) so isolation is never silently skipped. This directory is
	// what BOTH the SubagentStart/Stop hook subprocess's cmd.Dir (invoker.go) AND the
	// subagent's own Bash tool_use commands (via runSubagent's subCfg.Cwd below) actually
	// execute in — a prior version only reported subCwd in hook JSON payloads while every
	// subprocess still silently ran in the PARENT's real directory, so isolation="worktree"
	// was observable in transcripts/payloads but had NO effect on where anything actually
	// ran (caught via an empirical real-claude cwd probe: real claude's subagent process
	// itself reports a different `pwd`, which this mock did not reproduce). Any other
	// isolation value shares the parent cwd.
	// sr:docs https://code.claude.com/docs/en/sub-agents
	subCwd := cfg.Cwd
	if in.Isolation == "worktree" {
		subCwd = filepath.Join(cfg.Cwd, ".claude", "worktrees", "agent-"+agentID)
		if err := bindWorktree(ctx, cfg.Cwd, subCwd); err != nil {
			fmt.Fprintf(cfg.Stderr, "claude-mock: isolation=worktree: bind %s: %v (falling back to a plain directory)\n", subCwd, err)
			if mkErr := os.MkdirAll(subCwd, 0o755); mkErr != nil {
				fmt.Fprintf(cfg.Stderr, "claude-mock: isolation=worktree: mkdir %s: %v — isolation NOT applied, sharing parent cwd\n", subCwd, mkErr)
				subCwd = cfg.Cwd
			}
		}
	}

	// The subagent gets its OWN sidechain transcript under the parent session's
	// subagents/ dir — mirroring the real claude layout, whose subagent transcript's
	// first record IS the dispatch prompt. Seed it before SubagentStart so a hook can
	// read the prompt (e.g. an embedded `--task-id`) from transcript_path. configDir
	// is resolved through CLAUDE_CONFIG_DIR so tests stay isolated. The transcript's
	// project dir keys off the PARENT cwd (real claude nests subagents/ under the
	// parent session's transcript dir); only the recorded `cwd` field is the subagent's.
	// sr:docs https://code.claude.com/docs/en/agent-sdk/sessions (CLAUDE_CONFIG_DIR + projects/<encoded-cwd>)
	configDir := resolveConfigDir(cfg.ConfigDir)
	transcriptPath := seedSubagentTranscript(configDir, cfg.Cwd, subCwd, cfg.SessionID, agentID, agentType, toolUseID, in.Prompt)

	// SubagentStart — cannot block; a blocking error here is treated as a hard
	// failure of the Agent tool (the real claude never proceeds past a refused start).
	// sr:docs https://code.claude.com/docs/en/hooks#subagentstart
	// transcript_path is the SESSION's (the invoker's default) — real Claude Code
	// names the sub-agent by agent_id, and its own file only on SubagentStop.
	if _, err := inv.Fire(ctx, hooks.Input{
		SessionID:     cfg.SessionID,
		Cwd:           subCwd,
		HookEventName: hooks.EventSubagentStart,
		AgentType:     agentType,
		AgentID:       agentID,
	}); err != nil {
		return toolexec.Result{Output: fmt.Sprintf("Agent: SubagentStart blocked: %v", err), IsError: true}, agentID
	}
	sub := subagentRun{
		parent: cfg, subCwd: subCwd, agentID: agentID, agentType: agentType,
		sidechain: transcriptPath, parentReported: tr.reported,
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
	// sr:docs https://code.claude.com/docs/en/hooks#subagentstop
	blockCap := stopHookBlockCap() // 0 = unlimited
	finalText := sub.run(ctx, scriptPath, in.Prompt)
	for turn := 0; ; turn++ {
		// What the hook said is recorded as it fires — into the SUB-AGENT's own
		// file, where real Claude Code writes a SubagentStop's feedback and
		// attachment — before deciding whether to loop, so the last refusal
		// before the cap is on the record too.
		blocked, reason, _, _ := fireSubagentStop(ctx, cfg, subCwd, sub.invoker(inv), agentType, agentID, transcriptPath, turn > 0)
		if !blocked {
			break // clean stop — verify passed or the task was parked by the executor
		}
		if blockCap > 0 && turn >= blockCap {
			fmt.Fprintf(cfg.Stderr, "claude-mock: SubagentStop still blocked after %d turns (cap) — giving up\n", blockCap)
			break
		}
		fmt.Fprintf(cfg.Stderr, "claude-mock: SubagentStop blocked (%s) — re-running subagent (turn %d)\n", reason, turn+1)
		finalText = sub.run(ctx, scriptPath, in.Prompt)
	}

	return toolexec.Result{Output: buildAgentResultContent(agentID, agentType, finalText)}, agentID
}

// subagentRun is one dispatched sub-agent: where it runs, what it is, and the
// two transcripts involved — its own sidechain file, which its records go to,
// and the session's, which its hooks are told about.
type subagentRun struct {
	parent         Config
	subCwd         string
	agentID        string
	agentType      string
	sidechain      string
	parentReported string
}

// run drives one turn of the sub-agent's script as a nested run writing its
// sidechain file, and returns its final text.
func (s subagentRun) run(ctx context.Context, scriptPath, prompt string) string {
	if scriptPath == "" {
		return "no subagent script"
	}
	var buf bytes.Buffer
	subCfg := Config{
		ScriptPath:            scriptPath,
		SessionID:             s.parent.SessionID,
		AgentID:               s.agentID,
		AgentType:             s.agentType,
		IsResume:              true,
		Prompt:                prompt,
		Cwd:                   s.subCwd,
		ProjectDir:            s.parent.ProjectDir,
		ConfigDir:             s.parent.ConfigDir,
		PluginCacheDir:        s.parent.PluginCacheDir,
		Stderr:                s.parent.Stderr,
		Out:                   &buf,
		SuppressSubagentHooks: true,
		SidechainPath:         s.sidechain,
		ParentTranscriptPath:  s.parentReported,
	}
	if err := Run(ctx, subCfg); err != nil {
		fmt.Fprintf(s.parent.Stderr, "claude-mock: subagent run error: %v\n", err)
	}
	return lastResultText(buf.Bytes())
}

// invoker is inv recording into the sub-agent's own file. Opened afresh each
// time so its chain continues from whatever the nested run last wrote.
func (s subagentRun) invoker(inv *hooks.Invoker) *hooks.Invoker {
	side, err := openTranscript(s.sidechain, s.parentReported, recordStamp{
		SessionID: s.parent.SessionID, Cwd: s.subCwd, IsSidechain: true, AgentID: s.agentID,
	})
	if err != nil {
		return inv
	}
	return inv.WithRecorder(func(in hooks.Input, runs []hooks.HandlerRun) {
		side.recordHookRuns(in, runs)
		side.Close()
	})
}

// bindWorktree makes worktreeDir a REAL, usable directory for isolation="worktree": a genuine
// `git worktree add` of parentCwd's current HEAD when parentCwd is a git repo with at least one
// commit (mirroring real claude's own mechanism — a real worktree, same repo, isolated files),
// or a plain empty directory otherwise (parentCwd isn't a git repo, or has no commits yet — a
// worktree needs a HEAD to branch from). The caller (runAgentTool) already falls back to a plain
// mkdir on any error this returns, so this only needs to try the real thing and report failure.
func bindWorktree(ctx context.Context, parentCwd, worktreeDir string) error {
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
	// A detached worktree (no new branch) at the current HEAD — the subagent's own commits
	// inside it are exactly what a real isolation="worktree" dispatch is FOR (e.g. a spec-applier
	// authoring impl+spec that a10n-checks later drains from this directory).
	add := exec.CommandContext(ctx, "git", "-C", parentCwd, "worktree", "add", "--detach", worktreeDir, "HEAD")
	var stderr bytes.Buffer
	add.Stderr = &stderr
	if err := add.Run(); err != nil {
		return fmt.Errorf("git worktree add: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// resolveSubagentScript picks the subagent scenario script: the Agent input's
// `script` field wins, then the A10N_MOCK_SUBAGENT_SCRIPT env var, else "".
func resolveSubagentScript(fromInput string) string {
	if fromInput != "" {
		return fromInput
	}
	return os.Getenv(envSubagentScript)
}

// fireSubagentStop fires SubagentStop with the agent_id (+ the subagent's
// transcript_path, as the real hook payload carries) and returns the (possibly
// blocking) error from the hook so the caller can react to a block.
// sr:docs https://code.claude.com/docs/en/hooks#subagentstop
// fireSubagentStop fires the SubagentStop hook and reports whether it BLOCKED.
// A block is signalled two ways, both meaning "re-run the turn": the hook exits
// 2 (returned as a non-nil error from Fire), OR it exits 0 with a
// {"decision":"block"} stdout frame (the SubagentStop contract real Claude
// honours — exit-0 + decision, not a process error). The caller loops on a block.
// subCwd is the subagent's OWN cwd (its isolated worktree under isolation="worktree",
// else == cfg.Cwd) — reported in the SubagentStop payload's `cwd` field so a hook sees
// the isolated cwd, matching real claude. The mock's internal machinery (session file,
// block loop, hook invoker) stays on cfg since the mock does not bind a real worktree.
//
// The raw hooks.Output and the fire error are returned alongside the verdict
// because the caller records the refusal as a transcript attachment, and the two
// blocking forms carry their text differently: exit 2 puts it on the error,
// exit-0 {"decision":"block"} puts it on out.Reason. emitStopHookAttachment
// already knows how to read either, so both are handed over intact rather than
// flattened into the reason string — which would lose the decision field and
// mis-classify an exit-0 block as a bare message.
func fireSubagentStop(ctx context.Context, cfg Config, subCwd string, inv *hooks.Invoker, agentType, agentID, transcriptPath string, stopHookActive bool) (blocked bool, reason string, out hooks.Output, fireErr error) {
	out, err := inv.Fire(ctx, hooks.Input{
		SessionID:           cfg.SessionID,
		Cwd:                 subCwd,
		AgentTranscriptPath: transcriptPath,
		HookEventName:       hooks.EventSubagentStop,
		StopReason:          "end_turn",
		AgentType:           agentType,
		AgentID:             agentID,
		StopHookActive:      stopHookActive,
	})
	if err != nil {
		return true, err.Error(), out, err
	}
	if out.Decision == "block" {
		return true, out.Reason, out, nil
	}
	return false, "", out, nil
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
// sr:docs https://code.claude.com/docs/en/sub-agents
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
