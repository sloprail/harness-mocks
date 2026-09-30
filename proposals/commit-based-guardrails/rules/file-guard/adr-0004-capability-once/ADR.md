---
status: proposed
---

# ADR-0004: each mocked capability is implemented once, in core

**Concern.** Behaviour that several harnesses share: a Stop hook that can
block the turn, a PreToolUse refusal, compaction, background tasks, sub-agents.
The mocks for Claude Code, Cursor and Codex will mostly model the same
capabilities with different wire formats. Implementing each once per harness
triples the code and lets the copies drift.

**Decision.**
- A capability's behaviour lives once, in `core/<capability>/`, marked
  `// sr:capability <id>`.
- A harness mock holds only its adapter: how that harness names the
  capability's events, encodes its payloads and flags, and which capabilities
  it provides at all. The adapter code is marked `// sr:provides <id>
  <harness>`.
- An adapter translates. It decides nothing the capability decides (when a
  hook fires, whether a block is honoured, what order things happen in).

**Consequences.** A new harness is a new adapter plus a column in the
capability × harness table. Behaviour one harness genuinely does differently
becomes a parameter of the core capability, not a fork of it.
