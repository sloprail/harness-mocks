# Proposal: commit-based guardrails for the invariants and ADR funnels

**Status: for review. Not active.** These rules are written for the
commit-based sloprail engine that is being built now: file-guards that judge a
committed changeset, split into `subjects:`, with `SR_TREE` pointing at the
commit. They live here, not in `.sloprail/`, because today's engine would
load them without complaint (it accepts the unknown `subjects:` key) and run
them with per-file semantics, feeding the judges empty `{{ subject }}` and
`{{ changeset }}` values. Once the engine lands, activating them is one move:
`rules/` becomes `.sloprail/`.

```
proposals/commit-based-guardrails/
├── rules/                       laid out exactly as .sloprail/
│   ├── _lib/                    shared helpers and the shared ADR rubric (not rules)
│   ├── file-guard/              10 file-guards
│   └── gate/                    1 gate
└── tests/run.sh                 proves every deterministic part refuses and passes (37 cases)
```

Legend: **[G]** grounded (needs the user's words on the commit,
`Sloprail-Cites-User:`) · **[D]** deterministic script · **[P]** judge.

## The invariants funnel

```
user's words ─[G]─► invariants ─[G]─► coverage matrix ─[D]─► tests exist ─[P]─► tests do what the item says
                    stage 1            stage 2               stage 3            stage 4
```

| rule | kind | unit | what it holds |
|---|---|---|---|
| `invariant-grounded` | [G][P] | one per invariant added, changed or removed | the statement says what the cited words ask, no more and no less |
| `matrix-grounded` | [G][P] | one per matrix item added, changed or removed | the item's `case`/`expect` follows from the invariants it covers, and the words agree it's worth covering |
| `matrix-covered` | [D] | whole committed tree | every item has a `// sr:proves <item>` test · every invariant is in an item and has `// sr:invariant <id>` code · no marker names an unknown id · proofs only in `*_test.go` |
| `test-matches-item` | [P] | one per item whose entry or proving test changed | a proving test sets up the case and asserts the expected outcome |

The catalogs:

```yaml
# spec/invariants.yaml
invariants:
  - id: stop.block-cap
    statement: >-
      A Stop block continues the turn at most CLAUDE_CODE_STOP_HOOK_BLOCK_CAP
      times in a row (default 8; 0 means no cap); the next block is overridden.

# spec/matrix.yaml
items:
  - id: stop.block-cap.default
    covers: [stop.block-cap]
    case: {cap_env: unset, stop_hook: always-blocks}
    expect: Stop fires 9 times, the 9th block is overridden, and the result streams "".
```

```go
// sr:invariant stop.block-cap          ← on the code that upholds it
// sr:proves stop.block-cap.default     ← on the test that proves the item
```

## The ADR funnel

Each architectural decision is its own rule folder. `ADR.md` holds the
decision, and the rule's native `match` is where the decision applies. The ADR
and its enforcement are one thing, so they can't drift apart. The architecture
grows only by adding a folder, and only when the user's words ask for it.

| rule | kind | unit | what it holds |
|---|---|---|---|
| `adr-grounded` | [G][P] | one per ADR folder touched | a new ADR, a changed decision, a new exception or a loosened check needs the user's words, and they ask for that decision. Shrinking an exception list is waived. |
| `adr-undeclared` | [P] | one per changed Go file | the change implements no cross-cutting concern that lacks an ADR. If it does, it fails and the decision goes to the user. |
| `adr-0001-file-size` | [D] + gate | changed `.go` files | ≤150 lines (tests ≤400). Legacy files are listed, and may not grow. Limits and the list live in `ADR.md`, read by the script. |
| `adr-0002-layering` | [D] | whole module (`go list`) | `core/**` imports no mock, and no mock imports another mock |
| `adr-0003-subprocess-env` | [D] + [P] | changed Go files | only `core/procenv` assigns `cmd.Env`. Five legacy sites are listed. The judge catches an env built any other way. |
| `adr-0004-capability-once` | [D] + [P] | whole tree + changed files | one `// sr:capability <id>` per id, under `core/`. `// sr:provides <id> <harness>` only under that harness's mock. The judge checks that adapter code only translates. |

ADR-0001 through 0004 are **proposed**. Each needs your approval, which
`adr-grounded` will enforce by asking for your words once active. ADR-0003 is
the "five places, the sixth different" case taken from this repo:
`runner.go`, `stream.go`, `background.go`, `hooks/invoker.go` and
`toolexec.go` each build a child process's environment their own way, and
commit `0a093af` fixed one of them drifting.

## What these rules assume the engine provides

| assumed | used by |
|---|---|
| a Changeset payload: `.changeset.{base, head, commits[].trailers, files[], others[], citations[]}` | every rule |
| `SR_TREE`: a read-only checkout of the commit being judged | every script that reads beyond the diff |
| `subjects:` → `{"subjects":[{id, files, context}]}`, and `{{ subject }}` in templates | the 4 grounded or judged funnel rules, and `adr-undeclared` |
| `require: citation` resolved from `Sloprail-Cites-User:` trailers into `.changeset.citations` | `invariant-grounded`, `matrix-grounded`, `adr-grounded` |
| a check path may point outside the rule's folder (`../../_lib/…`) | the ADR judges, and the file-size gate |

## Running the tests

```
proposals/commit-based-guardrails/tests/run.sh
```

It builds throwaway git trees and Changeset payloads, and calls each script
the way the engine will. It covers every deterministic check, both directions,
and the subjects scripts (they decide what each judge sees). Two cases prove the
library fails closed: no `SR_TREE`, and a `git grep` that errors. The judges
themselves need the engine and are not exercised.

## Open questions for review

1. **The first ADRs.** Are 0001–0004 right? What's missing (e.g. hook-fire
   through one invoker, transcript-write through one writer, harness wire
   format only in adapters)?
2. **Limits.** 150 lines for code and 400 for tests, in ADR-0001.
3. **`adr-undeclared` scope.** Every changed non-test Go file under `core/`
   and `*-mock/` gets one judge. Is that too broad or too narrow?
4. **The capability catalog.** Harness-mocks' capabilities could be the
   invariants in `spec/invariants.yaml`, with a harness dimension in the
   matrix. Or they could get a separate `capabilities.yaml` × `providers.yaml`
   join table (use case 4 of the design).
