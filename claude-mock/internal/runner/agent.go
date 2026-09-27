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
//  5. Return a tool_result whose content carries the literal `agentId: <id>`
//     plus the sub-agent's final result text.
//
// toolUseID is the id of the Agent/Task tool_use that spawned the sub-agent;
// real Claude Code records it as the toolUseId of the sub-agent's .meta.json.
//
// sr:docs https://code.claude.com/docs/en/sub-agents
// sr:docs https://code.claude.com/docs/en/hooks#subagentstart
func runAgentTool(ctx context.Context, cfg Config, inv *hooks.Invoker, toolUseID string, rawInput json.RawMessage, tr *transcript) toolexec.Result {
	sub, _, errRes := prepareSubagent(cfg, toolUseID, rawInput, tr, false)
	if sub == nil {
		return errRes
	}
	out := sub.execute(ctx, inv, cfg.bg, sub.prompt)
	return toolexec.Result{Output: buildAgentResultContent(sub.agentID, sub.agentType, out.finalText)}
}

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
	if in.Isolation == "worktree" {
		subCwd = filepath.Join(cfg.Cwd, ".claude", "worktrees", "agent-"+agentID)
		if err := bindWorktree(context.Background(), cfg.Cwd, subCwd); err != nil {
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
	seedSubagentTranscript(sidechain, cfg.Cwd, subCwd, cfg.SessionID, agentID, agentType, toolUseID, in.Description, in.Prompt)

	return &subagentRun{
		parent: cfg, subCwd: subCwd, agentID: agentID, agentType: agentType,
		sidechain: sidechain, parentReported: tr.reported, sessionFile: sessionFile,
		script: resolveSubagentScript(in.Script), prompt: in.Prompt, background: background,
	}, in, toolexec.Result{}
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
	sessionFile    string
	script         string
	prompt         string
	background     bool
}

// subagentOutcome is how a sub-agent's run ended.
type subagentOutcome struct {
	finalText string
	failure   string
	toolUses  int
}

// execute fires SubagentStart, runs the sub-agent (re-running it while
// SubagentStop blocks) and returns how it ended.
func (s *subagentRun) execute(ctx context.Context, inv *hooks.Invoker, bg *backgroundTasks, prompt string) subagentOutcome {
	sideInv := s.invoker(inv)
	// SubagentStart — cannot block. transcript_path is the SESSION's (the
	// invoker's default); the sub-agent is named by agent_id.
	_, _ = sideInv.Fire(ctx, hooks.Input{
		SessionID:     s.parent.SessionID,
		Cwd:           s.subCwd,
		HookEventName: hooks.EventSubagentStart,
		AgentType:     s.agentType,
		AgentID:       s.agentID,
	})

	blockCap := stopHookBlockCap() // 0 = unlimited
	out := s.run(ctx, bg, prompt)
	for turn := 0; ; turn++ {
		// What the hook said is recorded as it fires — into the SUB-AGENT's own
		// file, where real Claude Code writes a SubagentStop's feedback — before
		// deciding whether to loop, so the last refusal before the cap is on the
		// record too.
		blocked, reason := fireSubagentStop(ctx, s, sideInv, bg, out.lastAssistant, turn > 0)
		if !blocked {
			break
		}
		if blockCap > 0 && turn >= blockCap {
			fmt.Fprintf(s.parent.Stderr, "claude-mock: SubagentStop still blocked after %d turns (cap) — giving up\n", blockCap)
			break
		}
		fmt.Fprintf(s.parent.Stderr, "claude-mock: SubagentStop blocked (%s) — re-running subagent (turn %d)\n", reason, turn+1)
		next := s.run(ctx, bg, prompt)
		next.toolUses += out.toolUses
		out = next
	}
	return subagentOutcome{finalText: out.finalText, failure: out.failure, toolUses: out.toolUses}
}

// runOutcome is one nested run of a sub-agent's script.
type runOutcome struct {
	finalText     string
	lastAssistant string
	failure       string
	toolUses      int
}

// run drives the sub-agent's script as a nested run writing its sidechain
// file, and reports its final text, its last assistant text, how many tools
// it called, and why it failed if it did.
func (s *subagentRun) run(ctx context.Context, bg *backgroundTasks, prompt string) runOutcome {
	if s.script == "" {
		return runOutcome{finalText: "no subagent script"}
	}
	var buf bytes.Buffer
	subCfg := Config{
		ScriptPath:            s.script,
		SessionID:             s.parent.SessionID,
		AgentID:               s.agentID,
		AgentType:             s.agentType,
		IsResume:              true,
		Prompt:                prompt,
		Cwd:                   s.subCwd,
		ProjectDir:            s.parent.ProjectDir,
		ConfigDir:             s.parent.ConfigDir,
		PluginCacheDir:        s.parent.PluginCacheDir,
		Model:                 s.parent.Model,
		Stderr:                s.parent.Stderr,
		Out:                   &buf,
		SuppressSubagentHooks: true,
		SyncSubagent:          !s.background,
		SidechainPath:         s.sidechain,
		ParentTranscriptPath:  s.parentReported,
		bg:                    bg,
		sessionFile:           s.sessionFile,
	}
	out := runOutcome{}
	if err := Run(ctx, subCfg); err != nil {
		fmt.Fprintf(s.parent.Stderr, "claude-mock: subagent run error: %v\n", err)
		out.failure = err.Error()
	}
	out.finalText = lastResultText(buf.Bytes())
	out.lastAssistant = lastAssistantText(buf.Bytes())
	out.toolUses = countToolUses(buf.Bytes())
	return out
}

// invoker is inv recording into the sub-agent's own file — real Claude Code
// writes a sub-agent's SubagentStart and SubagentStop attachments there, never
// into the session's. The file is opened only when a hook actually ran, and
// closed again right after.
func (s *subagentRun) invoker(inv *hooks.Invoker) *hooks.Invoker {
	stamp := newRecordStamp(s.parent.SessionID, s.subCwd)
	stamp.IsSidechain, stamp.AgentID = true, s.agentID
	return inv.WithRecorder(func(in hooks.Input, runs []hooks.HandlerRun) {
		side, err := openTranscript(s.sidechain, s.parentReported, stamp)
		if err != nil {
			return
		}
		defer side.Close()
		side.recordHookRuns(in, runs)
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

// fireSubagentStop fires SubagentStop and reports whether it BLOCKED — by
// exiting 2 or by an exit-0 {"decision":"block"} — and why. The payload is the
// real one: the session's transcript_path, the sub-agent's own
// agent_transcript_path, agent_id/agent_type, the sub-agent's cwd (its
// isolated worktree under isolation="worktree"), stop_hook_active on every
// re-fire after a block, last_assistant_message, and the session's
// background_tasks and session_crons.
// sr:docs https://code.claude.com/docs/en/hooks#subagentstop
func fireSubagentStop(ctx context.Context, s *subagentRun, inv *hooks.Invoker, bg *backgroundTasks, lastAssistant string, stopHookActive bool) (bool, string) {
	active := stopHookActive
	tasks := bg.running()
	crons := []any{}
	out, err := inv.Fire(ctx, hooks.Input{
		SessionID:            s.parent.SessionID,
		Cwd:                  s.subCwd,
		AgentTranscriptPath:  s.sidechain,
		HookEventName:        hooks.EventSubagentStop,
		AgentType:            s.agentType,
		AgentID:              s.agentID,
		StopHookActive:       &active,
		LastAssistantMessage: &lastAssistant,
		BackgroundTasks:      &tasks,
		SessionCrons:         &crons,
	})
	if err != nil {
		return true, err.Error()
	}
	if out.Decision == "block" {
		return true, out.Reason
	}
	return false, ""
}

// lastAssistantText is the text of the last assistant record in captured
// JSONL that has any — what real Claude Code sends as last_assistant_message.
func lastAssistantText(out []byte) string {
	text := ""
	for _, line := range bytes.Split(out, []byte{'\n'}) {
		if t := assistantText(line); t != "" {
			text = t
		}
	}
	return text
}

// assistantText joins the text blocks of an assistant record, or "".
func assistantText(line []byte) string {
	var rec struct {
		Type    string `json:"type"`
		Message *struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &rec) != nil || rec.Type != "assistant" || rec.Message == nil {
		return ""
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(rec.Message.Content, &blocks) != nil {
		var s string
		if json.Unmarshal(rec.Message.Content, &s) == nil {
			return s
		}
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// countToolUses counts the tool_use blocks in captured JSONL.
func countToolUses(out []byte) int {
	n := 0
	for _, line := range bytes.Split(out, []byte{'\n'}) {
		_, name, _ := extractFirstToolUseWithID(line)
		if name != "" {
			n++
		}
	}
	return n
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
