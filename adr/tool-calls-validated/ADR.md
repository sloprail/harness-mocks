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

- Every mock (claude, codex, cursor) checks each tool call its scenario script
  asks for against that harness's recorded tool schema (internal/toolspec)
  before playing it, in replays and ordinary runs.
- A call that fails the check fails the run, except where a recording shows the
  real harness answering that call itself.
