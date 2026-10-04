---
concern: which version a snapshot is frozen at, for the harness binary and for its doc pages
sloprails: [file-guard/snapshots-current, file-guard/capability-grounded, file-guard/capability-rigor]
---

# A recording is frozen by its harness version, a doc page by its hash; neither re-judges the other

## Concern

A harness's binary and its documentation change on separate clocks. A binary
update does not mean a doc page changed, and a re-fetched page that hashes the
same does not mean the harness did anything new. One version number for both
made every binary update re-judge every capability.

## Decision

- The harness binary a recording was made with is the run's own: `version` in
  `<harness>-mock/snapshots/runs/<name>/run.yaml`. The MANIFEST carries no
  version of the harness's; its `pin` only names which binary `capture.sh`
  runs next.
- A doc page is frozen by its own `sha256` and the `fetched` date in the
  MANIFEST. Nothing ties a run's version, or a page, to `pin` or to each other.
- Only a doc page whose `sha256` changed (added, removed or re-hashed) puts the
  capabilities that cite it back in judgement. A changed `pin`, a changed
  `fetched` date, or a re-freeze that finds the same `sha256` invalidates
  nothing, and no verdict's key carries a binary version.
- A run's samples are all recorded by the version in its `run.yaml`:
  `capture.sh run` refuses to add a sample of another version to a run unless
  it re-records the run.
- `capture.sh` runs the pinned binary, never the global one: it is installed
  at the exact pin into a cache of its own (`tools/harness-bin`), and one
  that does not report exactly the pin is refused. The user's own login is
  linked in; no credential is copied.
