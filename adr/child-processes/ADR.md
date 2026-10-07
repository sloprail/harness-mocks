---
concern: how every child process a mock starts is spawned, what environment it gets, and how it is stopped
sloprails: [file-guard/child-processes, file-guard/adr-conformance]
# Files that still spawn a process or set its environment themselves. None
# adds a site; the list only shrinks.
exceptions:
  - claude-mock/internal/hooks/invoker_exec.go
  - claude-mock/internal/hooks/plugin_cache.go
  - claude-mock/internal/runner/agent_prepare.go
  - claude-mock/internal/runner/background_bash.go
  - claude-mock/internal/runner/runner_print.go
  - claude-mock/internal/runner/stream_turn.go
  - claude-mock/internal/runner/transcript_stamp.go
  - claude-mock/internal/toolexec/toolexec_bash.go
---

# One package starts every child process

## Concern

Every child process a mock starts: hook commands, the Bash tool, the scenario
script, background tasks, and external tools such as git. A harness exports the
same session facts to all of them (for Claude Code: `CLAUDE_CODE_SESSION_ID`,
`CLAUDECODE`, `CLAUDE_CODE_ENTRYPOINT`, …).

## Decision

- Every child process is started by `internal/procexec`, from the kind of
  process, the session, and the harness's environment facts.
- The facts a harness exports come from that harness's adapter, which passes
  them to `internal/procexec`; no child process gets `os.Environ()` directly.
- Every child runs in its own process group, and stopping it kills the whole
  group, so no grandchild outlives the mock.
- No other code calls `exec.Command`, `exec.CommandContext` or assigns
  `cmd.Env`.
- One exception: claude's Bash tool (`procexec.Spec.LeaveGroup`) leaves a `&`
  job it started running past the call and past the mock's exit, neither
  waiting for it nor killing it, as real Claude Code does
  (`bash-background-job`). Only a timeout or cancel of the call itself kills
  its group. Hooks, scripts, other tools and the codex and cursor mocks keep
  the rule above.
- Where killing the group differs from what a harness does, the capability it
  affects records it under `deviations`, citing this ADR.
