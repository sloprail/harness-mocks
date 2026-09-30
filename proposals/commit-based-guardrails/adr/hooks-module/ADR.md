---
status: proposed
sloprails: [file-guard/module-boundaries, file-guard/concern-placement]
# A module ADR: the concern has a home, and only its api is imported from outside.
home: ["core/hooks/**"]
api: ["core/hooks"]
---

# The hooks module owns everything about hooks

## Concern

Hooks: reading hook settings (project, user, plugin), matching an event to its
handlers, running a handler command, and interpreting its exit code and JSON
output (block, allow, additional context). Today this is split between
`internal/hooks` and ad-hoc handling in the runner (for example, the Stop
block-cap logic and the `stop_hook_summary` record).

## Decision

- Everything about hooks lives under `core/hooks/`: settings, matching,
  invocation, and the meaning of a handler's result.
- Other code fires a hook only through the `core/hooks` package's API, and
  acts on the typed outcome it returns. It never reads a handler's exit code or
  stdout itself.
- Packages under `core/hooks/` other than `core/hooks` itself are internal to
  the module, and nothing outside it imports them.

## Consequences

A harness adapter supplies only its hook names and payload encoding.
Interpretation logic that leaks into the runner is moved back into the module.
