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
  capture.sh                     one per harness: run <scenario> | all (re-capture at the installed version)
  MANIFEST.yaml                  version · docs: {<page URL>: {version, sha256}}
  docs/<host>/<path>.md          doc pages pulled as markdown (<url>.md), once per page, shared by every capability;
                                 the path is a pure function of the URL: …/docs/en/hooks#x → docs/code.claude.com/docs/en/hooks.md
  runs/<name>/setup/             the scenario, hand-authored: prompt.txt · settings.json · hook.sh · args
  runs/<name>/run.yaml           version, command (written by capture.sh)
  runs/<name>/samples/<ts>/      each capture, timestamped: events.jsonl (normalized) · raw payloads/stream/transcript · SEAL
internal/<module>/
  module.yaml                    home · api
  candidates.sh                  owns the search for its logic: prints path:line:snippet
adr/<kebab-name>/ADR.md          concern · sloprails · modules · exceptions · settings (limits, space); ## Concern ## Decision
rules/                           → .sloprail/  (20 file-guards, 3 gates, structure.yaml)
  schemas/*.cue                  one CUE schema per structured file kind (invariant, capability, ADR, module, MANIFEST, run)
CAPABILITY-MAP.md                what claude-mock models today: 47 capabilities → code, tests, evidence, docs; proposed modules; gaps
tests/judge-cases/               judge inputs with their expected verdicts
```

**Go layout:** one module (`github.com/sloprail/harness-mocks`) with one binary per harness.
Each `<harness>-mock/` is a `package main` plus its adapter. The shared code lives in the
repo-root `internal/<module>/`: every mock in the repo imports it directly, and Go keeps anyone
outside the repo from importing it. Moving today's `claude-mock/internal/*` there is a
relocation that `tools/moveonly --rename` can check. Separating the Claude-specific parts into
the adapter is design work, done one module at a time.

Markers each take one token, per the engine's marker grammar:

```go
// sr:invariant <id>              code upholding an invariant
// sr:proves <id>                 a test proving an invariant
// sr:capability <id>             a capability's one implementation, in internal/
// sr:provides <id>/<harness>     that harness's adapter for it
// sr:proves <id>/<harness>       a test proving it for that harness
```

Legend: **D** deterministic · **P** judge · **G** needs the user's words on
the commit (`Sloprail-Cites-User:`).

## Rules

| | rule | kind | holds |
|---|---|---|---|
| **shapes** | `shapes` (file-guard) + `shapes` (gate) | D | each structured file matches its CUE schema (`rules/schemas/`) through `sr-file validate`: before the write (pending bytes) and on the committed bytes. Closed definitions, so an unknown key (`status`, a typo) is refused; a capability's runs must sit under its own harness. The other rules keep only what spans files. |
| **layout** | `structure.yaml` | gate | files land only in declared places: specs, ADRs, `internal/<module>/`, a mock's entrypoint, `internal/`, `e2e/NNN_suite/` and `e2etest/`, snapshot `setup/` and `capture.sh`, `tools/`, repo files. `claude-mock/evidence/` is frozen. Verified with today's engine (`sr-session pre-tool`), so it can be switched on before the rest. |
| **invariants** | `invariant-grounded` | G+P | the statement is what the user's words ask |
| | `invariant-covered` | D | ≥1 `sr:invariant` site and ≥1 `sr:proves` test; no unknown ids |
| | `invariant-rigor` | P | per invariant touched: its statement and ALL its tests. Each test proves it, and together they cover every condition, edge and failure path |
| **capabilities** | `capability-grounded` | P (+G to add/drop one) | each provider's cited doc sections say what the statement says |
| | `capability-covered` | D | one `sr:capability` in core; `sr:provides` and ≥1 proving test for each cell that isn't `false`; every mock has a cell (`false`, never omitted) |
| | `capability-rigor` | P | **the real-vs-mock comparison.** Per capability × harness touched, the e2e tests marked `sr:proves <capability>/<harness>` are compared with what the real harness did: the recorded runs' events (inlined) and the cited doc sections. Tests must reproduce the recorded situation and assert the recorded outcome, or a declared deviation. |
| | `snapshots-read-only` | gate | a Write, Edit or visible shell write under `snapshots/` is refused, pointing at `capture.sh`. `setup/` and `capture.sh` stay editable. |
| | `snapshots-current` | D | runs and docs match `version` (a bump makes them stale); each sample matches its `SEAL` and each doc its recorded sha256, so a hand edit is caught at the commit; samples are timestamped, have `events.jsonl`, and aren't duplicates; every cited URL, anchor and run resolves; an uncited run fails; `capture.sh` exists |
| **ADRs** | `adr-linked` | D | kebab name, one-line `concern`, no `status`, links ≥1 existing rule, linked modules exist, has the sections |
| | `adr-well-formed` | P | one concern, checkable, present tense only (no history, commit references or plans) |
| | `adr-matches-sloprails` | P | the ADR and its linked rules say the same thing |
| | `adr-grounded` | G+P | an ADR changes only with the user's words (shrinking `exceptions` is waived); a `module.yaml` is the agent's own statement and needs none |
| | `concern-undeclared` | P (Opus, `size-xl`) | once per changeset: a line makes the first instance of a choice the next author will copy or contradict (a policy, a mechanism, a global dependency), and no ADR decides it. The rubric teaches that test, not a list. |
| **modules** | `module-coverage` | D | every non-test Go file in the `space` (from `adr/modules-cover-code`) lies in exactly one module's home; homes don't overlap; legacy code is covered by `exceptions` globs, which only shrink |
| | `module-boundaries` | D | nothing imports past a module's `api` |
| | `module-leaks` | D→P | see below |
| **ADR-specific** | `file-size` (+ gate), `layering`, `child-processes`, `config-at-entry` | D | each ADR's deterministic half; settings and exceptions are read from the linked ADR. `child-processes` and `config-at-entry` share one helper (`_lib/confine.sh`: "only X may do Y; a legacy file may not add sites"). |
| | `adr-conformance` | P | **one** judge for every ADR's judged half: a changeset is judged once against every ADR that lists this rule in `sloprails` (today `capability-once`, `child-processes`, `mock-config`), not once per ADR |
| **refactoring** | `move-only` | D | every commit carrying `Sloprail-Refactor: move-only` is checked against its parent by `tools/moveonly`: only where code lives may change. A relocation names its dirs: `move-only <old>=<new>`. |

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

## Capabilities of claude-mock today

`spec/capabilities/` holds all 47 capabilities the mock models, mapped in
[`CAPABILITY-MAP.md`](CAPABILITY-MAP.md). **15 of them fail the capability schema** because
they cite no doc section or no recorded run. These are the grounding gaps to close, by
capturing a run with `capture.sh` or finding the doc, or else dropping the capability:

- **no recorded run (11):** hook-matcher-filter, hooks-all-matching-run, hook-timeout,
  http-hooks, plugin-hooks, worktree-hooks, subprocess-session-env, file-tools,
  schedule-wakeup, foreground-subagent-bash-ends-with-response, agent-input-validation
- **no doc (5, one overlapping):** hook-output-transcript-records,
  compaction-transcript-continuity, transcript-record-envelope,
  empty-tool-result-placeholder, agent-input-validation

## Tests mapped to capabilities

Every existing test that really proves a capability carries `// sr:proves <capability>/claude`: 167 markers
on 124 tests, each checked against the test body, not its name. [`TEST-COVERAGE.md`](TEST-COVERAGE.md)
has the table, the tests left unmarked and why, and the contradictions found. Uncovered:
`http-hooks` (no test at all) and `foreground-subagent-bash-ends-with-response`.

## Samples

- **Capabilities:** `stop-block-cap`, `pretooluse-refusal` and
  `manual-compaction`, citing real anchors of the Claude Code hooks page and
  the runs recorded today in `claude-mock/evidence/`.
- **Invariants:** `scenario-prompt-env` and `prompt-context-appended`: the
  mock's own features.
- **Module:** `internal/hooks` (sample). Its `candidates.sh` finds 24 candidates in today's code, all of them in tests, which the rule drops.
- **Snapshots:** `MANIFEST.yaml`, the `capture.sh` for Claude Code, and one authored scenario, `runs/cap/setup/` (a Stop hook that always blocks). `capture.sh` was run against a stub `claude` and the live hooks page, and its output passes `snapshots-current`, while a hand edit to a sample or the doc fails it.
- **ADRs (global only):** `file-size`, `layering`, `modules-cover-code`,
  `capability-once`, `child-processes` (one package spawns every child: its
  environment, its own process group, killed as a group) and `mock-config`
  (flags and env read once, at the mock's entrypoint).
- **Modules** decide their own concern, home and api in `internal/<m>/module.yaml`
  (11 target modules), not in ADRs.
- **Deviations:** where the mock deliberately differs from the real harness, the
  capability's cell says so: `deviations: [{adr, statement}]`. Changing one needs
  the user's words; the rigor judge then holds tests to the deviation.

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
   `runs/<name>/samples/<ts>/`, plus an `events.jsonl`. `capture.sh`'s
   normalizer produces it. A real run is not replayed against the mock: the model, cwd,
   env and tools differ, so the mock is proven by e2e tests and `capability-rigor`
   compares those tests with the recordings.
3. **Parked:** dimensions and suites.
