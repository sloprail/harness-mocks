#!/usr/bin/env bash
set -euo pipefail

# Fail closed, part two: each lookup of capability-rigor's prepare.sh (what the judge is handed) refuses, with
# a reason naming what could not be read, when it fails: a cell's cited runs and docs, the listing of a run's
# samples, the assembly of the runs, the listing of the tests that prove the pair, the assembly of a doc, the
# deviations, the count of the subjects and the final output. (The statement read, the context assembly and
# the pairs are refuses-when-a-lookup-fails'.) The CI path, no agent turn: `sr-checks run` judges committed
# ranges; the judge is a mock that always passes, so a refusal here is the rule's own machinery, never a
# verdict. The failures are injected with a jq shim, first on PATH for one sr-checks run only, that exits
# non-zero when an argument contains SHIM_JQ_FAIL or equals SHIM_JQ_FAIL_EXACT, in capability-rigor's own scripts only (many
# rules run in one sr-checks run, some with the same programs), after letting the first SHIM_JQ_SKIP such calls through: inputs-ready.sh, ahead of prepare.sh, runs some of the same programs, and
# is let through to reach prepare.sh's. Each scenario is its own commit (a verdict is cached by content, and
# a stored refusal is replayed).
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
mkdir -p bin
# the case's PATH is minimal: the rules' own tool, yq, is linked in from where the machine has it
for p in /opt/homebrew/bin/yq /usr/local/bin/yq /usr/bin/yq; do [ -x "$p" ] && ln -sf "$p" bin/yq && break; done
# a page of the vendor's docs: prepare.sh reads the frozen text, fetched on a miss
printf '#!/bin/sh\nout=""; while [ $# -gt 0 ]; do [ "$1" = -o ] && out="$2"; shift; done\nprintf "# s\\nfrozen text\\n" >"$out"\n' >bin/curl
chmod +x bin/curl
mkdir -p shim
printf '#!/bin/bash\nhit=""\nfor a in "$@"; do\n  case "$a" in *"${SHIM_JQ_FAIL:-@@none@@}"*) hit=1 ;; esac\n  [ -n "${SHIM_JQ_FAIL_EXACT:-}" ] && [ "$a" = "$SHIM_JQ_FAIL_EXACT" ] && hit=1\ndone\ncase "${SR_GUARDRAIL_DIR:-}" in *"${SHIM_JQ_IN:-}"*) ;; *) hit="" ;; esac\nif [ -n "$hit" ]; then\n  n=$(cat "$SHIM_JQ_COUNT" 2>/dev/null || echo 0); n=$((n + 1)); echo "$n" >"$SHIM_JQ_COUNT"\n  [ "$n" -gt "${SHIM_JQ_SKIP:-0}" ] && exit 5\nfi\nexec "%s" "$@"\n' "$(command -v jq)" >shim/jq
chmod +x shim/jq
export PATH="$PWD/bin:$PATH"
SHA="$(printf '# s\nfrozen text\n' | shasum -a 256 | cut -d' ' -f1)"
git init -q .
printf 'bin\nshim\n' >.gitignore
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
printf 'disabled:\n  - sloprail/gate/ci-verify-required\n  - sloprail/file-guard/rule-tests-pass\n' >.sloprail/config.yaml
mkdir -p spec/capabilities claude-mock/snapshots/runs/c/samples/20240101-000000 claude-mock/e2e
printf 'pin: "1"\ndocs:\n  https://d.example/p:\n    sha256: %s\n    fetched: "2026-10-01"\n' "$SHA" >claude-mock/snapshots/MANIFEST.yaml
printf 'version: 1\n' >claude-mock/snapshots/runs/c/run.yaml
printf '{"e":1}\n' >claude-mock/snapshots/runs/c/samples/20240101-000000/events.jsonl
(cd claude-mock/snapshots/runs/c/samples/20240101-000000 && shasum -a 256 ./events.jsonl >SEAL)
printf 'package e2e\n\n// sr:proves c/claude\nfunc TestC() {}\n' >claude-mock/e2e/c_test.go
# the harness has a replay exception list (empty): every run replays
mkdir -p claude-mock/e2e/018_replay; printf 'package e2e\n\nvar notReplaying = map[string]string{\n}\n' >claude-mock/e2e/018_replay/replay_allowlist_test.go
cap() { printf 'statement: %s\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/c]\n' "$1" >spec/capabilities/c.yaml; }
cap "c works"
git add -A && git commit -q -m base
BASE=$(git rev-parse HEAD)
export SR_CHECKS_JUDGE_MOCKS='{"file-guard/capability-rigor/tests-prove-as-documented":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'

n=0
scenario() {   # a new commit on a new branch from BASE: c's statement changes, which touches its pair
  n=$((n + 1)); git checkout -q -b "s$n" "$BASE"; cap "c works, differently $n"; git add -A && git commit -q -m "c's statement changes ($n)"
}
run_rule() {   # [ENV=VALUE...]
  : > "$SR_EVENTS_FILE"
  env "$@" sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_passed() {   # LABEL
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: capability-rigor did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING
  jq -es --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .rule=="capability-rigor" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: capability-rigor did not refuse with a reason saying '$2' (sr-checks exit $ran)" >&2; exit 1; }
}
inject() {   # MARKER [SKIP] — the lookups whose jq program carries MARKER fail, after SKIP of them went through, for this run only
  rm -f "$TMPDIR/shim.count"
  run_rule "PATH=$PWD/shim:$PATH" "SHIM_JQ_FAIL=$1" "SHIM_JQ_SKIP=${2:-0}" "SHIM_JQ_COUNT=$TMPDIR/shim.count" SHIM_JQ_IN=capability-rigor
}
inject_exact() {   # PROGRAM [SKIP] — the lookups whose jq program is exactly PROGRAM fail, after SKIP of them went through, for this run only
  rm -f "$TMPDIR/shim.count"
  run_rule "PATH=$PWD/shim:$PATH" "SHIM_JQ_FAIL=" "SHIM_JQ_FAIL_EXACT=$1" "SHIM_JQ_SKIP=${2:-0}" "SHIM_JQ_COUNT=$TMPDIR/shim.count" SHIM_JQ_IN=capability-rigor
}

# control: nothing injected, the statement change is judged (by the mock) and passes
scenario; run_rule SHIM_JQ_FAIL=
expect_passed "control"

# the cell's cited runs and docs, read again by prepare.sh (inputs-ready.sh read them first, with the same programs)
scenario; inject '(.runs // [])[]' 1
expect_refused "the cited runs listing fails (prepare)" "c/claude: its cited runs could not be listed, so it could not be prepared for the judge"
scenario; inject '(.docs // [])[]' 1
expect_refused "the cited docs listing fails (prepare)" "c/claude: its cited docs could not be listed, so it could not be prepared for the judge"

# a run: its samples listed (the `jq -R` of the first pipeline; the exception list's own `jq -R` goes first and is let through), the runs assembled
scenario; inject_exact '-R' 1
expect_refused "the samples listing fails" "c/claude: the samples of claude-mock/snapshots/runs/c could not be listed, so it could not be prepared for the judge"
scenario; inject 'samples: $sm[0]'
expect_refused "the runs assembly fails" "c/claude: the run claude-mock/snapshots/runs/c could not be listed, so it could not be prepared for the judge"

# the tests that prove the pair
scenario; inject 'map(select(. != ""))'
expect_refused "the tests listing fails" "c/claude: the tests proving it could not be listed, so it could not be prepared for the judge"

# a doc assembled, the deviations read
scenario; inject '{ref: $r, path: $p, line:'
expect_refused "the doc assembly fails" "c/claude: the cited doc https://d.example/p#s could not be listed, so it could not be prepared for the judge"
scenario; inject '.deviations // []'
expect_refused "the deviations read fails" "c/claude: its deviations could not be read, so it could not be prepared for the judge"

# the count of the subjects, and the output
scenario; inject_exact 'length'
expect_refused "the subject count fails" "the prepared subjects could not be counted, so nothing could be handed to the judge"
scenario; inject '{additionalContext: {subjects: .}}'
expect_refused "the output write fails" "the prepared subjects could not be written, so nothing could be handed to the judge"

# The lookups the CI path cannot reach on its own: subjects.sh runs first and would refuse first for a failure
# it shares, so inputs-ready.sh and prepare.sh are run as the engine runs them, over the payload
# `sr-checks changeset` prints for the subject, in the rule's own environment. A refusal is {"reason"} on stdout.
# The failures: the touched pairs (rigor_pairs), the capability markers (their jq read) and the diff of a
# modified file that carries a marker (a git shim: a diff that cannot be had is not "no changed lines").
git checkout -q -b direct "$BASE"
mkdir -p internal
printf 'package core\n\n// sr:capability c\nfunc C() {}\n' >internal/c.go
git add -A && git commit -q -m "c, with its marker" && DBASE=$(git rev-parse HEAD)
printf 'package e2e\n\n// sr:proves c/claude\nfunc TestC() { _ = 1 }\n' >claude-mock/e2e/c_test.go
git add -A && git commit -q -m "c's proving test changes"
sr-checks changeset --rule capability-rigor --base "$DBASE" --head HEAD | jq -c '.subjects[0].payload' >direct.payload
jq -e '.changeset.files | length > 0' direct.payload >/dev/null || { echo "no payload for the direct scripts" >&2; exit 1; }
printf '#!/bin/bash\ncase " $* " in *" diff "*) [ -n "${SHIM_GIT_DIFF_FAIL:-}" ] && exit 5 ;; esac\nexec "%s" "$@"\n' "$(command -v git)" >shim/git
chmod +x shim/git
direct() {   # SCRIPT [ENV=VALUE...] — the script over the payload; its stdout is left in $out, its status in $rc
  local s="$1"; shift
  out="$(cd .sloprail/file-guard/capability-rigor && env "$@" SR_TREE="$OLDPWD" SR_GUARDRAIL_DIR="$PWD" bash "./$s" <"$OLDPWD/direct.payload")" && rc=0 || rc=$?
}
expect_direct_refused() {   # LABEL SUBSTRING
  [ "$rc" -ne 0 ] && printf '%s' "$out" | jq -e --arg s "$2" '.reason | contains($s)' >/dev/null ||
    { echo "$1: not refused with a reason saying '$2' (exit $rc): $out" >&2; exit 1; }
}
direct inputs-ready.sh SHIM_JQ_FAIL=
[ "$rc" -eq 0 ] || { echo "control: inputs-ready.sh refused a ready input: $out" >&2; exit 1; }
direct prepare.sh SHIM_JQ_FAIL=
[ "$rc" -eq 0 ] && printf '%s' "$out" | jq -e '.additionalContext.subjects[0].id == "c/claude"' >/dev/null ||
  { echo "control: prepare.sh did not hand the judge the pair (exit $rc): $out" >&2; exit 1; }

for s in inputs-ready.sh prepare.sh; do
  case "$s" in
    inputs-ready.sh) tail_reason="so the judge's inputs could not be checked" ;;
    prepare.sh) tail_reason="so nothing could be prepared for the judge" ;;
  esac
  direct "$s" "PATH=$PWD/shim:$PATH" 'SHIM_JQ_FAIL=select($want == "" or $want == "\($id)/\($h)")' SHIM_JQ_IN=capability-rigor SHIM_JQ_COUNT="$TMPDIR/direct.count"
  expect_direct_refused "$s: the pairs lookup fails" "the touched capability pairs could not be worked out, $tail_reason"
  direct "$s" "PATH=$PWD/shim:$PATH" 'SHIM_JQ_FAIL=(.newMarkers // [])' SHIM_JQ_IN=capability-rigor SHIM_JQ_COUNT="$TMPDIR/direct.count"
  expect_direct_refused "$s: the markers read fails" "the capability markers this change touches could not be worked out, $tail_reason"
  direct "$s" "PATH=$PWD/shim:$PATH" SHIM_JQ_FAIL= SHIM_GIT_DIFF_FAIL=1
  expect_direct_refused "$s: the diff of a changed file fails" "the capability markers this change touches could not be worked out, $tail_reason"
done

# A run with no samples is a legitimately empty listing, not a failed one: the glob matches nothing, and a
# listing loop that ends on a false `[ -f ] &&` would fail the substitution under pipefail and refuse here.
# prepare.sh, over the payload of a change citing such a run, must hand the judge the run with `samples: []`
# (inputs-ready.sh may report its own "no sealed sample"; it is not run here).
mkdir -p claude-mock/snapshots/runs/empty
printf 'version: 1\n' >claude-mock/snapshots/runs/empty/run.yaml
printf 'statement: c works\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/empty]\n' >spec/capabilities/c.yaml
git add -A && git commit -q -m "c cites a run with no samples"
sr-checks changeset --rule capability-rigor --base "$DBASE" --head HEAD | jq -c '.subjects[0].payload' >direct.payload
direct prepare.sh SHIM_JQ_FAIL=
[ "$rc" -eq 0 ] && printf '%s' "$out" | jq -e '.additionalContext.subjects[0].context.runs[0].samples == []' >/dev/null ||
  { echo "a cited run with no samples: prepare.sh refused or lost the run (exit $rc): $out" >&2; exit 1; }
