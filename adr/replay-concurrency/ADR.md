---
concern: how many recorded-run replays a mock's replay test runs at once
sloprails: [file-guard/adr-conformance]
---

# Replays run with limited concurrency

## Concern

A recording's hooks run under wall-clock limits (SessionEnd's default is one
second, and a recorded hook sleeps half of it behind a shell and
`git rev-parse`). A machine loaded by many replays at once kills a hook the real
run finished, and the replay goes red without the mock being wrong:
`session-end-hook-failure` and `session-end-hook-output` were red with sixteen
replays at once and green alone.

## Decision

- Replays run with limited concurrency so hook timeouts are not hit under load.
- Each mock's table-driven replay test takes a slot from the core's gate
  (`internal/replay`: `NewGate`, `Hold`) before it runs a replay; the gate's
  width is half the CPUs, between one and four. No test starts a replay
  without a slot.
- A recording whose replay is red only under load is not listed in the
  exception list as flaky: the width is what makes it replay.
