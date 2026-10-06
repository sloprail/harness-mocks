---
concern: how the list of recorded runs that do not replay green and its reasons are kept from hiding differences
sloprails: [file-guard/replay-exceptions-only-shrink]
---

# The replay exception list may only shrink

## Concern

The recorded runs a mock's replay does not reproduce yet are listed with a
reason in the `notReplaying` map of that mock's `replay_allowlist_test.go`
(every such file in the repo); a `flaky:` entry is one whose replay is green in
some runs and not in others. A list that can grow, or whose reasons can be
softened, turns every difference between a recording and its mock into an
accepted one.

## Decision

- A change to the `notReplaying` map of any `replay_allowlist_test.go` removes
  keys and never adds one: adding an entry fails CI (the
  `replay-exceptions-only-shrink` file-guard compares the map's keys at the base
  and at the head). A new `flaky:` entry is an addition.
- An existing entry's reason never moves to a weaker category: a reason starts
  with one of `adapter:`, `mock gap:`, `untriaged:` or `flaky:`; `flaky:` is the
  weakest, `untriaged:` is weaker than the triaged `adapter:` and `mock gap:`, a
  move between the triaged ones, or from `flaky:` or `untriaged:` to a stronger
  category, is allowed, and no entry takes a reason that starts with none of
  the four (a reason that has none today may stay as it is).

## Source

The user's words for the second bullet: "A replay exception's reason may not
move to a weaker category (flaky < untriaged < triaged), and a flaky entry runs
3 times and fails if never green; it is never skipped."
