---
concern: what a mock persists of a session
sloprails: [file-guard/adr-conformance]
---

# A mock persists a session exactly as the real harness does for the same flags

## Concern

What `claude-mock`, `codex-mock` and `cursor-mock` leave on disk of a session
(its transcript or rollout), and what their hooks are told of it. Our system
relies on the persisted trajectory, so a mock that keeps a session the real
harness does not, or drops one it keeps, misleads whatever reads it.

## Decision

- A mock persists a session exactly as the real harness does for the same
  flags: where the recording shows a transcript, the mock keeps one; where the
  recording shows none (for `codex exec --ephemeral`: no rollout under
  `CODEX_HOME`, and `transcript_path` null in every hook payload), the mock
  keeps nothing and its hooks say null. A scratch file a mock needs while it
  runs is removed when it exits.
