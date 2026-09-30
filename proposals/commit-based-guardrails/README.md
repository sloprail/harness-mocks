# Proposal: commit-based guardrails for the invariants and ADR funnels

**Status: for review. Not active.** These rules are written for the
commit-based sloprail engine being built now: file-guards that judge a
committed changeset, split into `subjects:`, with `SR_TREE` pointing at the
commit. They live here, not in `.sloprail/`, because today's engine loads them
without complaint (it accepts the unknown `subjects:` key), runs them with
per-file semantics, and feeds the judges empty `{{ subject }}` and
`{{ changeset }}`. Once the engine lands, activating them is three moves:

| here | becomes |
|---|---|
| `rules/` | `.sloprail/` |
| `adr/` | `adr/` (repo root) |
| `spec/` | `spec/` (repo root, with real content) |

```
proposals/commit-based-guardrails/
├── adr/<kebab-name>/ADR.md     the architecture: 5 proposed ADRs, each linking its sloprails
├── spec/                       SAMPLE catalogs: invariants, coverage matrix, capabilities
├── rules/                      laid out exactly as .sloprail/
│   ├── _lib/                   shared helpers and the shared ADR rubric (not rules)
│   ├── file-guard/             14 file-guards
│   └── gate/                   1 gate
└── tests/run.sh                every deterministic part, refusing and passing (51 cases)
```

Legend: **[G]** grounded (needs the user's words on the commit,
`Sloprail-Cites-User:`) · **[D]** deterministic script · **[P]** judge.

---

## The invariants funnel

```
user's words ─[G]─► invariants ─[G]─► coverage matrix ─[D]─► tests exist ─[P]─► tests do what the item says
                    stage 1            stage 2               stage 3            stage 4
```

| rule | kind | unit | what it holds |
|---|---|---|---|
| `invariant-grounded` | [G][P] | one per invariant added, changed or removed | the statement says what the cited words ask, no more and no less |
| `matrix-grounded` | [G][P] | one per matrix item added, changed or removed | `case`/`expect` follow from the invariants covered, and the words agree the case is worth covering |
| `matrix-covered` | [D] | whole committed tree | every item has a `// sr:proves <item>` test · every invariant is in an item and has `// sr:invariant <id>` code · no marker names an unknown id · proofs only in `*_test.go` |
| `test-matches-item` | [P] | one per item whose entry or proving test changed | the proving test sets up the case and asserts the expected outcome |

**Samples:**
- [`spec/invariants.yaml`](spec/invariants.yaml) holds 4 invariants, all real
  Claude Code 2.1.282 behaviour from `claude-mock/EVIDENCE.md`: the Stop
  block cap, `CLAUDE_CODE_SESSION_ID` in every child process, the PreToolUse
  refusal, and the compaction order.
- [`spec/matrix.yaml`](spec/matrix.yaml) holds 8 items, including an
  interaction case (compaction after a refused tool).

Markers in code look like this:

```go
// sr:invariant stop.block-cap          ← on the code that upholds it
// sr:proves stop.block-cap.default     ← on the test function that proves the item
```

---

## The ADR funnel

ADRs live at `adr/<kebab-name>/ADR.md`. Each has frontmatter linking the
sloprails that enforce it, and three fixed sections:

```markdown
---
status: proposed | accepted | superseded
sloprails: [gate/file-size, file-guard/file-size]   # ≥1, each <nature>/<name>
home: ["core/hooks/**"]      # optional: where this concern lives (module ADRs)
api:  ["core/hooks"]         # optional: what others may import from the home
exceptions: [...]            # optional: legacy paths; the list only shrinks
limits: {...}                # optional: settings the linked rules read
---
# Title
## Concern
## Decision
## Consequences
```

**Links are many-to-many.** A rule finds the ADRs it enforces by its own
qualified name, so nothing inside a rule names an ADR:
- `file-size` reads its limits and exceptions from whichever ADR links it;
- one rule (`concern-placement`) enforces several ADRs;
- one ADR (`file-size`) is enforced by a gate plus a file-guard.

### Rules about ADRs themselves

| rule | kind | what it holds |
|---|---|---|
| `adr-linked` | [D] | every ADR is kebab-named, links ≥1 sloprail, and every link resolves to a rule folder. Status, list types and the three sections are checked. Unparseable frontmatter is refused, not skipped. It re-runs when a rule changes, so deleting a linked rule fails. |
| `adr-matches-sloprails` | [P] | one per ADR whose text, or any linked rule, changed. The judge sees the ADR and every file of every linked rule, and checks that each Decision bullet is enforced by some rule, that no rule refuses what no ADR decides, and that the matches cover what the decision governs. |
| `adr-well-formed` | [P] | one per changed `ADR.md`: one concern; the decision is a checkable rule naming the place or mechanism (no "prefer" or "where possible"); scoped; consequences stated; no contradictions with the frontmatter |
| `adr-grounded` | [G][P] | a new ADR, a changed decision, a new exception, a changed `home`/`api`, or a link added or dropped needs your words, and they must ask for that. Shrinking `exceptions` is waived. |
| `concern-placement` | [P] | one per changed Go file. **Undeclared:** it implements a cross-cutting concern no ADR decides → fail, and you decide. **Leak:** it implements a concern whose ADR has a `home`, outside that home → fail. |

### The proposed ADRs, and the rules that enforce them

| ADR | sloprails | deterministic part | judged part |
|---|---|---|---|
| `file-size` | `gate/file-size`, `file-guard/file-size` | ≤150 lines, tests ≤400; 17 legacy files may not grow. Checked before the write, and again at commit. | — |
| `layering` | `file-guard/layering` | `go list`: core imports no mock; no mock imports another | — |
| `subprocess-env` | `file-guard/subprocess-env` | only `core/procenv` assigns `cmd.Env`; 5 legacy files may not add any | an env built any other way |
| `capability-once` | `file-guard/capability-once` | `spec/capabilities.yaml` ⇄ markers, both directions (below) | adapter code only translates |
| `hooks-module` (sample module ADR) | `file-guard/module-boundaries`, `file-guard/concern-placement` | nothing outside `core/hooks/**` imports past its api `core/hooks` | hook logic inlined anywhere else is a leak |

---

## Answers to the review

**How do we prove each capability actually has markers?**
`capability-once`'s script checks the catalog against the code in both
directions, over the whole committed tree:
- each capability in [`spec/capabilities.yaml`](spec/capabilities.yaml) has
  exactly one `// sr:capability <id>`, under `core/`;
- each `supported` cell has a `// sr:provides <id> <harness>` under
  `<harness>-mock/`;
- every harness mock has a cell for every capability (a missing cell fails,
  and `n/a` needs a reason);
- a marker for an uncatalogued capability, or a provides in an `n/a` cell,
  fails.

Code that implements a capability with no marker at all can't be seen by a
marker check. That's `concern-placement`'s judge: capability behaviour outside
`core/` (the ADR's `home`) is a leak.

**Modules/DDD pieces that gravitate their stuff and don't leak.**
This is the same ADR mechanism, not a separate one. A module is an ADR with
`home` and `api` (sample: [`adr/hooks-module`](adr/hooks-module/ADR.md)).
Enforcement comes in two halves:
- **`module-boundaries` [D]:** nothing outside the home imports past the api.
  This is `go list`, and it's exact.
- **`concern-placement` [P]:** module logic re-implemented inline elsewhere.
  That isn't an import, so only a judge can see it.

`capability-once` is the same idea for capabilities (`home: core/**`).

**A judge for `ADR.md` itself.** `adr-well-formed` judges the content, and
`adr-linked` checks the format deterministically.

---

## What these rules assume the engine provides

| assumed | used by |
|---|---|
| a Changeset payload: `.changeset.{base, head, commits[].trailers, files[], others[], citations[]}` | every rule |
| `SR_TREE`: a read-only checkout of the commit being judged | every script that reads beyond the diff |
| `subjects:` → `{"subjects":[{id, files, context}]}`, and `{{ subject }}` in templates | all rules marked "one per" above |
| `require: citation` resolved from `Sloprail-Cites-User:` trailers into `.changeset.citations` | `invariant-grounded`, `matrix-grounded`, `adr-grounded` |
| a check path may point outside the rule's folder (`../../_lib/…`) | the ADR judges, the file-size gate |
| a `prepare` may return `{"skip": true}` | `_lib/linked-adrs.sh`, for a rule no ADR links |

## Running the tests

```
proposals/commit-based-guardrails/tests/run.sh
```

The harness builds throwaway git trees and Changeset payloads, and calls each
script the way the engine will. It also runs `adr-linked` over this proposal's
own `adr/` and `rules/`, so the sample ADRs are shown to be well-formed and
linked. The judges need the engine and are not exercised. Their subjects
scripts are, since those decide what each judge sees.

## Open questions

1. Are these the right first ADRs, and what's missing (e.g. transcript-write
   through one writer, harness wire format only in adapters)?
2. The 150/400 line limits.
3. Are capabilities invariants too? Today `spec/capabilities.yaml` and
   `spec/invariants.yaml` are separate. An invariant could name the
   capability it belongs to, making the matrix per (capability × harness).
