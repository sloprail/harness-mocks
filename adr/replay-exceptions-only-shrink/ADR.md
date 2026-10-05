---
concern: how the list of recorded runs that do not replay green is kept from growing
sloprails: [file-guard/replay-exceptions-only-shrink]
---

# The replay exception list may only shrink

## Concern

The recorded runs a mock's replay does not reproduce yet are listed with a
reason in the `notReplaying` map of
`codex-mock/e2e/001_hooks/replay_allowlist_test.go`. A list that can grow turns
every difference between a recording and its mock into an accepted one.

## Decision

- A change to the `notReplaying` map in
  `codex-mock/e2e/001_hooks/replay_allowlist_test.go` removes keys and never
  adds one: adding an entry fails CI (the `replay-exceptions-only-shrink`
  file-guard compares the map's keys at the base and at the head).
