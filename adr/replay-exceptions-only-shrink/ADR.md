---
concern: how the list of recorded runs that do not replay green is kept from growing
sloprails: [file-guard/adr-conformance]
---

# The replay exception list may only shrink

## Concern

The recorded runs a mock's replay does not reproduce yet are listed with a
reason (`notReplaying` in `codex-mock/e2e/001_hooks/replay_allowlist_test.go`).
A list that can grow turns every difference between a recording and its mock
into an accepted one.

## Decision

- The replay exception list may only shrink: adding an entry fails CI.
