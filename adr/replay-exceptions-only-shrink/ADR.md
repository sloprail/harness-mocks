---
concern: how the list of recorded runs that do not replay green is kept from growing
sloprails: [file-guard/replay-exceptions-only-shrink]
---

# The replay exception list may only shrink

## Concern

The recorded runs a mock's replay does not reproduce yet are listed with a
reason in the `notReplaying` map of that mock's `replay_allowlist_test.go`
(every such file in the repo). A list that can grow, or whose reasons can be softened, turns
every difference between a recording and its mock into an accepted one.

## Decision

- A change to the `notReplaying` map of any `replay_allowlist_test.go` removes
  keys and never adds one: adding an entry fails CI (the
  `replay-exceptions-only-shrink` file-guard compares the map's keys at the base
  and at the head). A new `flaky:` entry is an addition.
- An existing entry's reason never moves to a weaker category: `flaky:` is the
  weakest, `untriaged:` is weaker than every triaged reason, and a triaged
  reason is any reason that is neither `flaky:` nor `untriaged:` (`adapter:`,
  `mock gap:`, ...); a move between triaged reasons, or from `flaky:` or
  `untriaged:` to a stronger category, is allowed.
- A `flaky:` entry is never skipped: the generated replay test runs it three
  times and fails when none of them is green.
- Deleting a list file outright is not judged here: removing the whole list
  only shrinks it ("why fucking not? do" asked for the list to be guarded, and
  "The replay exception list may only shrink; adding an entry fails CI" is
  satisfied by a deletion).
- The file-guard compares the map's text, so it accepts only forms it can
  compare (one `"run": "reason",` per line, no escape inside a reason's
  category, the map named only in its own file); these restrictions exist so
  that the entry comparison can be trusted.

## Source

The user's words for the last two bullets: "A replay exception's reason may not
move to a weaker category (flaky < untriaged < triaged), and a flaky entry runs
3 times and fails if never green; it is never skipped."
