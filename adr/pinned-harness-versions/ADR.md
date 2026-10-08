---
concern: what a capability verdict's key is made of, and what puts a capability back in judgement
sloprails: [file-guard/capability-grounded, file-guard/capability-rigor]
---

# A capability verdict's key carries recordings, never a doc page or a binary version

## Concern

A harness binary's version and its documentation are separate things, and recordings own the truth (adr/capability-grounding: a recording outranks a doc): a doc page is pulled when a recording is captured, not on its own. So neither a doc page's change nor a binary update may put a capability back in judgement.

## Decision

- `file-guard/capability-grounded/subjects.sh` and `file-guard/capability-rigor/subjects.sh`, which compute the verdict key and which capabilities are judged, never read `<harness>-mock/snapshots/MANIFEST.yaml`: a doc re-freeze of a page, or a bumped `pin`, re-judges nothing, and the key carries no doc page's hash or text.
- The key never carries a harness binary version: the binary a recording was made with is recorded in the run's own `run.yaml`, not in the key.
