---
status: proposed
sloprails: [file-guard/layering]
---

# One shared core; harness mocks depend on it, never on each other

## Concern

Package dependencies between the shared core and the per-harness mocks.
Behaviour common to every harness (tool execution, the session store and
lifecycle, streaming, hook dispatch) exists once. A mock that imports another
mock is the first step to copying one harness's shape into another.

## Decision

- No package under `core/` imports a package under any `*-mock/`.
- No package under `<a>-mock/` imports a package under a different
  `<b>-mock/`. It may import `core/` and its own packages.

## Consequences

Anything two mocks both need moves to `core/` first.
