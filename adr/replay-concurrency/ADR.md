---
concern: how many recorded-run replays run at once
sloprails: [file-guard/adr-conformance]
---

# Replays run with limited concurrency

## Concern

How many recorded-run replays of a mock run at once. A recording's hooks run
under wall-clock limits (SessionEnd's default is one second), which a machine
loaded by many replays at once makes a mock miss, though the mock is right.

## Decision

- Replays run with limited concurrency so hook timeouts are not hit under load:
  the replay core's `Run` (`internal/replay`) holds one of at most
  `min(max(NumCPU/2, 1), 4)` process-wide slots for each replay, for every mock.
