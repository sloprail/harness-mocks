---
status: proposed
sloprails: [file-guard/capability-once]
home: ["core/**"]
---

# Each mocked capability is implemented once, in core

## Concern

Behaviour several harnesses share: a Stop hook that can block the turn, a
PreToolUse refusal, compaction, background tasks, sub-agents. The mocks for
Claude Code, Cursor and Codex will mostly model the same capabilities with
different wire formats. Implemented once per harness, the code triples and
the copies drift.

## Decision

- `spec/capabilities.yaml` lists every capability the mocks model, and which
  harness provides each one (`supported`, or `n/a` with a reason). Every
  harness mock has a cell for every capability.
- A capability's behaviour lives once, in `core/`, on code marked
  `// sr:capability <id>`.
- A harness mock holds only its adapter: how that harness names the
  capability's events, and encodes its payloads and flags. Adapter code is
  marked `// sr:provides <id> <harness>`, and exists exactly for the cells
  marked `supported`.
- An adapter translates. It decides nothing the capability decides: when a
  hook fires, whether a block is honoured, what order things happen in.

## Consequences

A new harness is a new adapter, plus a column of cells in
`spec/capabilities.yaml`. Behaviour one harness genuinely does differently
becomes a parameter of the core capability, not a fork of it.
