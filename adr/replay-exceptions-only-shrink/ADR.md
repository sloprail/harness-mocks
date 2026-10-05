---
concern: how the list of recorded runs that do not replay green is kept from growing
sloprails: [file-guard/replay-exceptions-only-shrink]
---

# The replay exception list may only shrink

## Concern

The recorded runs a mock's replay does not reproduce yet are listed with a
reason in the `notReplaying` map of that mock's `replay_allowlist_test.go`
(`claude-mock/e2e/018_replay/`, `codex-mock/e2e/001_hooks/`, and any mock's that
appears later). A list that can grow, or whose reasons can be softened, turns
every difference between a recording and its mock into an accepted one.

## Decision

- A change to the `notReplaying` map of any `replay_allowlist_test.go` removes
  keys and never adds one: adding an entry fails CI (the
  `replay-exceptions-only-shrink` file-guard compares the map's keys at the base
  and at the head). A new `flaky:` entry is an addition.
- An existing entry's reason never moves to a weaker category: `flaky:` is the
  weakest, `untriaged:` is weaker than any triaged reason (`adapter:`,
  `mock gap:`, ...). Triaging an entry (untriaged to a triaged reason) is
  allowed.
- A `flaky:` entry is never skipped: the generated replay test runs it three
  times and fails when none of them is green.
