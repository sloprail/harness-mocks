---
concern: which parts of a real harness a mock models, and how a capability states what it leaves out
sloprails: [file-guard/capability-rigor, file-guard/adr-conformance]
---

# A mock models the non-interactive harness its tests drive

## Concern

A real harness has more surface than its mock: interactive commands, tools
for other platforms, tools no test of ours drives. A capability's docs
describe all of it. Where the mock leaves part of it out, the gap must be
visible and deliberate, not a test nobody wrote.

## Decision

- A mock models the harness as its tests drive it: a non-interactive session
  (print mode, or a stream of prompts), with the tools those sessions use.
- Out of the model, for every harness:
  - what exists only in an interactive session: slash commands (no mock
    parses one), a status line command, and the terminal sessions (tmux) a
    harness opens;
  - every tool a mock's tool executor does not implement (for claude-mock,
    each tool name `claude-mock/internal/toolexec` does not handle);
  - every hook event a mock does not define or never fires (for claude-mock,
    each event not named in `claude-mock/internal/hooks/event.go`, and those it
    names but no code path fires);
  - the file-system effects a mock's scripted control record only announces
    (for claude-mock, the worktree directory behind `worktree_create` and
    `worktree_remove`: they fire the hooks, and no directory exists);
  - aborting a running tool: a mock never interrupts one, so what a harness
    reports for an abort (an interrupted result, `is_interrupt`) is out;
  - failures a mock's own runtime cannot produce (for claude-mock, a Bash
    whose shell will not start: it always runs `/bin/sh`);
  - the reference text a harness prints inside its own diagnostics (such as
    the output schema Claude Code appends to a hook validation error): a mock
    writes the diagnostic's first line and the hook's own output;
  - a background time limit, and a foreground command moving to the
    background.
- A capability whose docs describe behaviour on a part left out declares it in
  that harness's cell, as a `deviations` entry citing this ADR and naming the
  part. Its tests prove the rest.
- No `deviations` entry citing this ADR names a part its mock models (a tool
  its executor handles, an event it fires).

