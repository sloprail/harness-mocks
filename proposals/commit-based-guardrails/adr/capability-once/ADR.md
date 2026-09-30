---
concern: where a mocked harness capability is implemented
sloprails: [file-guard/capability-once]
home: ["core/**"]
---

# Each mocked capability is implemented once, in core

## Concern

Harness behaviour the mocks model: a Stop hook that blocks the turn, a
PreToolUse refusal, compaction, background tasks, sub-agents. Several harnesses
provide the same capability with different wire formats.

## Decision

- `spec/capabilities.yaml` lists every capability the mocks model, and for
  every harness mock whether it is `supported` or `n/a` with a reason.
- A capability's behaviour lives once, in `core/`, on code marked
  `// sr:capability <id>`.
- A harness mock holds only its adapter: that harness's event names, payload
  encoding and flags for the capability. Adapter code is marked
  `// sr:provides <id> <harness>`, and exists exactly for the `supported` cells.
- An adapter translates and decides nothing: when a hook fires, whether a
  block is honoured, and in what order things happen are the capability's.
- Behaviour that differs between harnesses is a parameter of the core
  capability, never a copy of it.
