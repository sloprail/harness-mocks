---
concern: which version a snapshot is frozen at, for the harness binary and for its doc pages
sloprails: [file-guard/snapshots-current, file-guard/capability-grounded, file-guard/capability-rigor]
---

# A recording carries its harness version, a doc page is frozen by its hash; a binary update re-judges nothing

## Concern

A harness binary's version and its documentation are separate things: a binary update does not mean a doc page changed. A binary version never stands in for a doc page's freshness, and never appears in a capability verdict's key.

## Decision

- A doc page is frozen by its own `sha256` in `<harness>-mock/snapshots/MANIFEST.yaml`. Only a page whose `sha256` changed (added, removed or re-hashed) puts the capabilities that cite it back in judgement; a binary update, or a re-freeze that finds the same `sha256`, invalidates nothing.
- The harness binary a recording was made with is the run's own: `version` in `<harness>-mock/snapshots/runs/<name>/run.yaml`, which `snapshots-current` requires. Harness binaries are pinned in a local cache, not the global install.
