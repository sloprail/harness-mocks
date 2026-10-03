---
concern: which module every piece of code belongs to
sloprails: [file-guard/module-coverage, file-guard/module-distinct]
# The code that must be mapped to modules.
space: ["internal/**", "*-mock/**"]
# Globs of code not yet in any module. Each only shrinks.
exceptions: []
---

# Every piece of code belongs to exactly one module

## Concern

Which module owns each non-test Go file, so every concern has one home and no
code sits outside a boundary.

## Decision

- Every non-test Go file matching `space` lies in the `home` of exactly one
  module (a `module.yaml`).
- Module homes do not overlap.
- Each module states its concern in the `concern:` line of its `module.yaml`.
  No two modules' `concern:` lines name the same responsibility, whether
  under different words or with one split across two modules.
- A module's `home` holds only code of its `concern:`. A file belongs to
  another concern when its main responsibility is what another module's
  `concern:` line names.
- Both are judged when a `module.yaml` is added or changed, against every
  other module's `concern:`, `home` and the files each `home` glob matches.
- Code matching `exceptions` belongs to no module; nothing new is added there.
