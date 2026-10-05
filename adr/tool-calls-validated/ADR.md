---
concern: what a mock does with a tool call its scenario script asks it to make
sloprails: [file-guard/adr-conformance]
---

# A mock validates the tool calls of its script before it plays them

## Concern

The tool calls a scenario script asks a mock to make. A call the mock does not
implement (a tool it lacks, a parameter it ignores, an option value it does not
act on) played anyway lets a wrong script or a wrong recording pass.

## Decision

- Every tool call a scenario script asks `claude-mock`, `codex-mock` or
  `cursor-mock` to make is checked by the core's validator (`internal/toolspec`)
  before the mock plays it, in replays and in ordinary runs alike.
- Each harness declares the tools its mock implements in one schema of its own
  (name, parameters, types, which are required, the option values the mock
  implements). Every tool and parameter in it is one a recorded run shows the
  real harness take; none is invented.
- The mock refuses, with an error that names the tool and the parameter, an
  unknown tool, an unknown parameter, a missing required parameter, a value of
  the wrong type, and an option value it does not implement.
- Only a mistake whose answer a recorded run shows may be answered instead of
  refused. The schema lists such a kind for a tool, with the run; the mock then
  answers exactly as recorded (the same frame and error text, the variable parts
  taken from the call), and a test proves each listed kind against its
  recording. A kind with no recording behind it is refused.
