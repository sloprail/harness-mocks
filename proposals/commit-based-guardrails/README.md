# Proposal: commit-based guardrails for invariants, capabilities and ADRs

**Status: for review. Not active.** Written for the commit-based sloprail
engine being built now: file-guards judge a committed changeset, split into
`subjects:`, with `SR_TREE` pointing at the commit. Today's engine would load
these rules silently (it accepts the unknown `subjects:` key) and run them
with the wrong semantics, so they live here. To activate, move `rules/` to
`.sloprail/`, and the other folders to the repo root.

```
spec/
  invariants/<id>.yaml           statement                                   features: the user's words
  capabilities/<id>.yaml         statement · providers.<harness>: {docs: [full URL#anchor], runs: [repo path]} | false
<harness>-mock/snapshots/        the real harness, frozen at one version; written ONLY by capture.sh
  capture.sh                     one per harness: run <scenario> | doc <url> | all (re-capture at the installed version)
  MANIFEST.yaml                  version · docs: {<page URL>: {version, sha256}}
  docs/<host>/<path>.md          doc pages pulled as markdown (<url>.md), once per page, shared by every capability;
                                 the path is a pure function of the URL: …/docs/en/hooks#x → docs/code.claude.com/docs/en/hooks.md
  runs/<name>/setup/             the scenario, hand-authored: prompt.txt · settings.json · hook.sh · args
  runs/<name>/run.yaml           version, command (written by capture.sh)
  runs/<name>/samples/<ts>/      each capture, timestamped: events.jsonl (normalized) · raw payloads/stream/transcript · SEAL
core/<module>/
  module.yaml                    home · api
  candidates.sh                  owns the search for its logic: prints path:line:snippet
adr/<kebab-name>/ADR.md          concern · sloprails · modules · exceptions · settings (limits, space); ## Concern ## Decision
rules/                           → .sloprail/  (19 file-guards, 2 gates, structure.yaml)
tests/judge-cases/               judge inputs with their expected verdicts
```

Markers each take one token, per the engine's marker grammar:

```go
// sr:invariant <id>              code upholding an invariant
// sr:proves <id>                 a test proving an invariant
// sr:capability <id>             a capability's one implementation, in core/
// sr:provides <id>/<harness>     that harness's adapter for it
// sr:proves <id>/<harness>       a test proving it for that harness
```

Legend: **D** deterministic · **P** judge · **G** needs the user's words on
the commit (`Sloprail-Cites-User:`).

## Rules

| | rule | kind | holds |
|---|---|---|---|
| **layout** | `structure.yaml` | gate | files land only in declared places: specs, ADRs, `core/<module>/`, a mock's entrypoint, `internal/`, `e2e/NNN_suite/` and `e2etest/`, snapshot `setup/` and `capture.sh`, `tools/`, repo files. `claude-mock/evidence/` is frozen. Verified with today's engine (`sr-session pre-tool`), so it can be switched on before the rest. |
| **invariants** | `invariant-grounded` | G+P | the statement is what the user's words ask |
| | `invariant-covered` | D | ≥1 `sr:invariant` site and ≥1 `sr:proves` test; no unknown ids |
| | `invariant-rigor` | P | per invariant touched: its statement and ALL its tests. Each test proves it, and together they cover every condition, edge and failure path |
| **capabilities** | `capability-grounded` | P (+G to add/drop one) | each provider's cited doc sections say what the statement says |
| | `capability-covered` | D | one `sr:capability` in core; `sr:provides` and ≥1 proving test for each cell that isn't `false`; every mock has a cell (`false`, never omitted) |
| | `capability-rigor` | P | per capability × harness touched: the tests prove it as that harness's docs and runs show it |
| | `snapshots-read-only` | gate | a Write, Edit or visible shell write under `snapshots/` is refused, pointing at `capture.sh`. `setup/` and `capture.sh` stay editable. |
| | `snapshots-current` | D | runs and docs match `version` (a bump makes them stale); each sample matches its `SEAL` and each doc its recorded sha256, so a hand edit is caught at the commit; samples are timestamped, have `events.jsonl`, and aren't duplicates; every cited URL, anchor and run resolves; an uncited run fails; `capture.sh` exists |
| | `fidelity-replay` | D | the cited runs, replayed against the mock by `tools/replay`, match a sample |
| **ADRs** | `adr-linked` | D | kebab name, one-line `concern`, no `status`, links ≥1 existing rule, linked modules exist, has the sections |
| | `adr-well-formed` | P | one concern, checkable, present tense only (no history, commit references or plans) |
| | `adr-matches-sloprails` | P | the ADR and its linked rules say the same thing |
| | `adr-grounded` | G+P | an ADR, or a `module.yaml`, changes only with the user's words (shrinking `exceptions` is waived) |
| | `concern-undeclared` | P | once per changeset, from an ADR index (id + concern): changed code implements a concern no ADR covers |
| **modules** | `module-coverage` | D | every non-test Go file in the `space` (from `adr/modules-cover-code`) lies in exactly one module's home; homes don't overlap; legacy code is covered by `exceptions` globs, which only shrink |
| | `module-boundaries` | D | nothing imports past a module's `api` |
| | `module-leaks` | D→P | see below |
| **ADR-specific** | `file-size` (+ gate), `layering`, `subprocess-env` | D | limits and exceptions are read from the linked ADR |

**`module-leaks`:**
1. Each module's `candidates.sh` owns the whole search for its logic, and
   prints every `path:line:snippet` that looks like it. The rule keeps only
   candidates on lines this range adds, or all of them when that module's
   `module.yaml` or `candidates.sh` changed.
2. It drops matches inside the module's home, in tests, and in paths the
   module's ADRs list as exceptions.
3. It judges only what's left: a leak, or just a use? With nothing left, it
   makes no judge call. Code outside the range was judged when its own range
   passed, so it isn't grepped again.

## Samples

- **Capabilities:** `stop-block-cap`, `pretooluse-refusal` and
  `manual-compaction`, citing real anchors of the Claude Code hooks page and
  the runs recorded today in `claude-mock/evidence/`.
- **Invariants:** `scenario-prompt-env` and `prompt-context-appended`: the
  mock's own features.
- **Module:** `core/hooks` (sample). Its `candidates.sh` finds 24 candidates in today's code, all of them in tests, which the rule drops.
- **Snapshots:** `MANIFEST.yaml`, the `capture.sh` for Claude Code, and one authored scenario, `runs/cap/setup/` (a Stop hook that always blocks). `capture.sh` was run against a stub `claude` and the live hooks page, and its output passes `snapshots-current`, while a hand edit to a sample or the doc fails it.
- **ADRs:** `file-size`, `layering`, `subprocess-env`, `capability-once` and
  `hooks-module` and `modules-cover-code`. They're for your approval.

## What the engine must provide

- a Changeset payload: `.changeset.{base, head, commits[].trailers, files[].{path, status, diff, …}, citations[]}`;
- `SR_TREE`;
- `subjects:`, and `{{ subject }}` in templates;
- `require: citation` resolved from `Sloprail-Cites-User:` trailers;
- check paths that point outside the rule's folder (`../../_lib/…`);
- `{"skip": true}` from a `prepare`.

## Open

1. **Snapshot docs:** copying Claude Code's doc pages into this public repo.
   Is that OK licence-wise, or should docs be snapshotted by content hash
   only, and fetched when a rule runs?
2. **Migrating `claude-mock/evidence/`:** each fixture becomes
   `runs/<name>/samples/<ts>/`, plus an `events.jsonl`. That needs the capture
   normalizer, and `tools/replay` for `fidelity-replay`.
3. **Parked:** dimensions and suites.
