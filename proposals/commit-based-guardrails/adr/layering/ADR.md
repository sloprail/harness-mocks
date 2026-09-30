---
concern: which packages core and the harness mocks may import
sloprails: [file-guard/layering]
---

# One shared core; harness mocks depend on it, never on each other

## Concern

Package imports between `core/` and the per-harness mocks.

## Decision

- No package under `core/` imports a package under any `*-mock/`.
- No package under `<a>-mock/` imports a package under a different
  `<b>-mock/`. It imports only `core/` and its own packages.
- Code two mocks both need lives in `core/`.
