---
concern: how a recorded run made with a flag the mock refuses is replayed
sloprails: [file-guard/adr-conformance]
---

# A refused-flag recording replays as a check that the mock refuses the flag

## Concern

A recorded run can be made with a flag its mock refuses (adr/fail-fast-unimplemented):
the run has no mock behaviour to compare, so it would otherwise sit on the list of
recordings that do not replay green.

## Decision

- A recorded run made with a flag the mock refuses replays as a check that the mock
  refuses that flag; it is not an exception-list entry.
