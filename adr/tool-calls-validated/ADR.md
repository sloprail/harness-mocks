---
concern: what a mock does with a tool call its scenario script asks it to make
sloprails: [file-guard/adr-conformance]
---

# A mock validates the tool calls of its script before it plays them

## Concern

The tool calls a scenario script asks a mock to make. The mock plays what the
script gives it, and a call it does not implement played anyway lets a wrong
script or a wrong recording pass.

## Decision

- Every tool call a scenario script asks `claude-mock`, `codex-mock` or
  `cursor-mock` to make is validated by the core (`internal/toolspec`) before the
  mock plays it, in replays and in ordinary runs alike.
- A call that fails validation is refused as `adr/fail-fast-unimplemented` says:
  loudly, with an error that names it.
