---
concern: how many recorded-run replays run at once
sloprails: [file-guard/adr-conformance]
---

# Replays run with limited concurrency

## Concern

How many recorded-run replays of a mock (the replay core's `Run`, which the
codex and claude replays go through) run at once. A recording's hooks run under
wall-clock limits (SessionEnd's default is one second), which a machine loaded by
many replays at once makes a mock miss, though the mock is right.

## Decision

- Replays run with limited concurrency so hook timeouts are not hit under load.
- The limit is held by the replay core (`internal/replay`): its `Run` takes a
  slot before a replay and gives it back after, for every mock.
