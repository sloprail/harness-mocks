#!/usr/bin/env bash
set -euo pipefail
# A capability is judged per harness: one harness's missing tests never refuse a change that touches only another
# harness. Capability c is provided by claude (its cited section shows it) and by codex (its cited section shows
# nothing of it, so the judge mock refuses codex's tests):
#   a recording of claude changes                          -> judged for claude alone, passed
#   a recording of codex changes                           -> judged for codex, refused
#   the shared sr:capability marker alone changes          -> no harness's pair is touched, nothing refused
#   the shared statement changes                           -> every harness's pair is judged, refused for codex
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
mkdir -p bin
# the case's PATH is minimal: the rules' own tool, yq, is linked in from where the machine has it
for p in /opt/homebrew/bin/yq /usr/local/bin/yq /usr/bin/yq ; do [ -x "$p" ] && ln -sf "$p" bin/yq && break; done
# a page of the vendor's docs: the judge's prepare.sh reads the frozen text, fetched on a miss
printf '#!/bin/sh\nout=""; while [ $# -gt 0 ]; do [ "$1" = -o ] && out="$2"; shift; done\nprintf "# s\\nfrozen text\\n" >"$out"\n' >bin/curl
chmod +x bin/curl
export PATH="$PWD/bin:$PATH"
SHA="$(printf '# s\nfrozen text\n' | shasum -a 256 | cut -d' ' -f1)"
git init -q .
printf 'bin\nout\n' >.gitignore
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
printf 'disabled:\n  - sloprail/gate/ci-verify-required\n  - sloprail/file-guard/rule-tests-pass\n' >.sloprail/config.yaml
mani() { printf 'pin: "1"\ndocs:\n  https://d.example/p:\n    sha256: %s\n    fetched: "2026-10-01"\n' "$SHA" >"$1-mock/snapshots/MANIFEST.yaml"; }
cap() { printf 'statement: %s\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/c]\n  codex:\n    docs: [https://d.example/p#undocumented]\n    runs: [codex-mock/snapshots/runs/c]\n' "$1" >spec/capabilities/c.yaml; }
mkdir -p spec/capabilities internal/c
for h in claude codex; do
  mkdir -p "$h-mock/snapshots/runs/c/samples/20240101-000000" "$h-mock/e2e"
  mani "$h"
  printf 'version: 1\n' >"$h-mock/snapshots/runs/c/run.yaml"
  printf '{"e":1}\n' >"$h-mock/snapshots/runs/c/samples/20240101-000000/events.jsonl"
  (cd "$h-mock/snapshots/runs/c/samples/20240101-000000" && shasum -a 256 ./events.jsonl >SEAL)
  printf '// sr:proves c/%s\n' "$h" >"$h-mock/e2e/c_test.go"
  mkdir -p "$h-mock/e2e/018_replay"; printf 'package e2e\n\nvar notReplaying = map[string]string{\n}\n' >"$h-mock/e2e/018_replay/replay_allowlist_test.go"
done
cap "c works"
printf '// sr:capability c\nfunc C() {}\n' >internal/c/c.go
git add -A && git commit -q -m base
BASE=$(git rev-parse HEAD)
export SR_CHECKS_JUDGE_MOCKS='{"file-guard/capability-rigor/tests-prove-as-documented":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'
run() { local status=0; sr-checks run --base "$BASE" --head HEAD >out 2>&1 || status=$?; return 0; }

# a recording of claude changes: only claude's pair is judged, and its tests hold up (codex's gap is not this change's)
git checkout -q -b rec-claude "$BASE"
: >"$SR_EVENTS_FILE"
printf '{"e":2}\n' >claude-mock/snapshots/runs/c/samples/20240101-000000/events.jsonl
git add -A && git commit -q -m "record c for claude again"
run
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length==1 and .[0].outcome=="passed"' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "a change that touches only claude was refused for codex's gap" >&2; exit 1; }

# a recording of codex changes: codex's pair is judged and refused
git checkout -q -b rec-codex "$BASE"
: >"$SR_EVENTS_FILE"
printf '{"e":2}\n' >codex-mock/snapshots/runs/c/samples/20240101-000000/events.jsonl
git add -A && git commit -q -m "record c for codex again"
run
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length==1 and .[0].outcome=="refused" and (.[0].reason | contains("do not drive the mock"))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "a change to codex's recording was not judged and refused" >&2; exit 1; }

# the shared sr:capability marker alone changes: it touches no harness's pair, so nothing is judged or refused
git checkout -q -b core "$BASE"
: >"$SR_EVENTS_FILE"
printf '// sr:capability c\nfunc C() { _ = 1 }\n' >internal/c/c.go
git add -A && git commit -q -m "the core function changes"
run
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "a change to the shared core marker alone was refused for a harness's tests" >&2; exit 1; }

# the shared statement changes: every harness's pair is judged, so codex's refuses it
git checkout -q -b statement "$BASE"
: >"$SR_EVENTS_FILE"
cap "c works, said another way"
git add -A && git commit -q -m "the statement changes"
run
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length==1 and .[0].outcome=="refused" and (.[0].reason | contains("do not drive the mock"))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "a change to the shared statement did not judge every harness" >&2; exit 1; }
