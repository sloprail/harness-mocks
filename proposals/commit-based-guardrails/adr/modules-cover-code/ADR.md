---
concern: which module every piece of code belongs to
sloprails: [file-guard/module-coverage]
# The code that must be mapped to modules.
space: ["core/**", "*-mock/**"]
# Globs of code not yet in any module. Each only shrinks.
exceptions: ["claude-mock/**"]
---

# Every piece of code belongs to exactly one module

## Concern

Which module owns each non-test Go file, so every concern has one home and no
code sits outside a boundary.

## Decision

- Every non-test Go file matching `space` lies in the `home` of exactly one
  module (a `module.yaml`).
- Module homes do not overlap.
- Code matching `exceptions` belongs to no module; nothing new is added there.
