---
concern: how many recorded-run replays run at once
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
