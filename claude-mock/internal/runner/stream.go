package runner

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/scenario"
)

// streamAndHook runs the scenario script in a turn-based loop, reads its JSONL
// output, fires hooks, executes tool calls, and writes valid lines to cfg.Out.
//
// Turn-based protocol:
//  1. Script runs and emits JSONL records.
//  2. Each forwarded record is appended to the session JSONL file so the script
//     can read the full conversation history on subsequent turns.
//  3. When the script emits an assistant record with a tool_use block, the mock
//     executes the tool locally, synthesises a user record with a tool_result
//     block, writes it to stdout AND the session file, then re-invokes the script.
//  4. The script receives A10N_MOCK_SESSION_FILE pointing at the session JSONL.
//     It can use any shell tool (grep/tail/jq) to inspect history and decide what
//     to emit next — no single-value env vars are needed.
//  5. Repeat until the script emits a result frame or exits.
//
// sr:docs https://code.claude.com/docs/en/env-vars (CLAUDE_CONFIG_DIR)
// sr:docs https://docs.anthropic.com/en/docs/claude-code/sdk#stream-json-output-format
// maxIdenticalTurns bounds how many times in a row the script may emit the SAME
// pending tool_use (same name+input). A static scenario that doesn't advance its
// output based on conversation history would otherwise loop forever (re-run →
// same tool_use → re-run …). Hitting the bound is a scenario bug, surfaced as an
// error instead of a hang.
//
// sr:invariant loop-guard
const maxIdenticalTurns = 5

// withEmptyResult is a result frame with its result text emptied.
func withEmptyResult(line []byte) []byte {
	var m map[string]any
	if json.Unmarshal(line, &m) != nil || m == nil {
		return line
	}
	m["result"] = ""
	if b, err := marshalRecord(m); err == nil {
		return b
	}
	return line
}

// writeCapOverride writes the warning real Claude Code records when it
// overrides a Stop block at the cap and ends the turn (claude 2.1.282, verbatim).
func writeCapOverride(cfg Config, tr *transcript, blocks int) {
	text := fmt.Sprintf("A hook blocked the turn from ending %d consecutive times — overriding and ending turn. ", blocks) +
		"For Stop/SubagentStop hooks, check stop_hook_active in the input and return success while it's true. Set CLAUDE_CODE_STOP_HOOK_BLOCK_CAP to raise this limit."
	tr.persistMap(map[string]any{
		"type": "system", "subtype": "informational", "content": text,
		"isMeta": false, "level": "warning",
	})
	// and the stream says so too (recorded: runs/cap)
	writeFrame(cfg, map[string]any{"type": "system", "subtype": "informational", "content": text, "level": "warning"})
	writeFrame(cfg, map[string]any{"type": "system", "subtype": "notification", "key": "stop-hook-block-cap", "text": text, "priority": "high", "color": "warning"})
}

// turnResult is one script invocation's outcome.
type turnResult struct {
	// done: the script ended its turn (result frame, or it exited having
	// emitted neither a tool_use nor a compaction).
	done bool
	// sig identifies what the invocation did when it did not end the turn (the
	// pending tool_use, or a compaction), for the loop guard.
	sig string
	// lastText is the text of the last assistant record it emitted.
	lastText string
	thinking bool // an assistant record held thinking and no text, since the last one that did
	// resultLine is the result frame that ended the turn, held back until Stop
	// has let the turn end.
	resultLine []byte
	// emptyReply: the turn's answer was a model response with no visible output (only thinking).
	emptyReply bool
}

// pendingToolUse carries the fields needed to execute a tool and synthesise the
// tool_result record.
type pendingToolUse struct {
	ToolUseID string
	ToolName  string
	ToolInput json.RawMessage

	// Blocked is set when a PreToolUse hook REFUSED this tool call — an exit-0
	// permissionDecision deny (or deprecated decision:block), or an exit 2. The
	// tool is NOT executed; BlockReason is what the refusal tool_result quotes.
	Blocked     bool
	BlockReason string

	// Invalid is the tool_use_error a call whose input failed validation is
	// answered with: no hook fired and the tool does not run.
	Invalid *toolexec.Result
}

// scanResult is what one script invocation's output amounted to.
type scanResult struct {
	pending    pendingToolUse
	group      []pendingToolUse // the calls of the message, in order, when it holds several (pending is the first)
	done       bool             // a result frame was seen
	compactSig string           // a compaction happened (and what it was)
	lastText   string           // text of the last assistant record
	thinking   bool             // an assistant record held thinking and no text since the last one that did
	resultLine []byte           // the result frame, not yet streamed
	execGate   *scenario.Gate   // what the step's call waits for before it is carried out (script's gate)
}

// buildEnv constructs the environment for a script invocation.
// A10N_MOCK_SESSION_FILE points at the session JSONL so the script can read the
// full conversation history with any shell tool.
// CLAUDE_CONFIG_DIR is set to the same dir so that tooling that reads Claude
// Code config also finds the mock's session files.
// sr:docs https://code.claude.com/docs/en/env-vars (CLAUDE_CONFIG_DIR)
func buildEnv(cfg Config, tr *transcript) []string {
	sessionPath := ""
	if tr != nil {
		// Opened (and created) here if nothing has been written yet: the script
		// reads history from it, and a script run always follows the prompt.
		if f := tr.file(); f != nil {
			sessionPath = f.Name()
		}
	}
	return append(os.Environ(),
		// CLAUDE_CODE_SESSION_ID mirrors the real claude CLI, which exports the active
		// session id into every Bash-tool subprocess. Tools that resolve "the current
		// session" (e.g. a10n-task-executor session autopilot) read it.
		// sr:docs https://code.claude.com/docs/en/env-vars (CLAUDE_CODE_SESSION_ID)
		"CLAUDE_CODE_SESSION_ID="+cfg.SessionID,
		"A10N_MOCK_SESSION_ID="+cfg.SessionID,
		// sr:invariant scenario-prompt-env
		"A10N_MOCK_PROMPT="+cfg.Prompt,
		// sr:invariant prompt-context-appended
		"A10N_MOCK_ADDITIONAL_CONTEXT="+cfg.AdditionalContext,
		"A10N_MOCK_IS_RESUME="+boolStr(cfg.IsResume),
		// sr:invariant session-file-env
		"A10N_MOCK_SESSION_FILE="+sessionPath,
		"CLAUDE_CONFIG_DIR="+cfg.ConfigDir,
	)
}
