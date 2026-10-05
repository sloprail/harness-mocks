#!/usr/bin/env bash
set -euo pipefail
# Docs follow recordings. capability-rigor judges a capability when its cell, a recording it cites or a test
# proving it changes, never for a doc re-freeze (a MANIFEST change):
#   a doc re-freeze alone                                  -> the rule is not even asked (no FileGuardChecked event)
#   a recording of a capability whose tests hold up        -> judged, passed
#   a recording of a capability whose cited section shows nothing of it -> judged, refused; fixed -> judged, passed
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
mani() { printf 'pin: "1"\ndocs:\n  https://d.example/p:\n    sha256: %s\n    fetched: "%s"\n' "$SHA" "$1" >claude-mock/snapshots/MANIFEST.yaml; }
cap() { printf 'statement: %s\nproviders:\n  claude:\n    docs: [https://d.example/p#%s]\n    runs: [claude-mock/snapshots/runs/%s]\n' "$2" "$3" "$1" >"spec/capabilities/$1.yaml"; }
mkdir -p spec/capabilities claude-mock/snapshots claude-mock/e2e
mani 2026-10-01
cap c "c works" s
cap d "d works" undocumented
for r in c d; do
  mkdir -p "claude-mock/snapshots/runs/$r/samples/20240101-000000"
  printf 'version: 1\n' >"claude-mock/snapshots/runs/$r/run.yaml"
  printf '{"e":1}\n' >"claude-mock/snapshots/runs/$r/samples/20240101-000000/events.jsonl"
  (cd "claude-mock/snapshots/runs/$r/samples/20240101-000000" && shasum -a 256 ./events.jsonl >SEAL)
  printf '// sr:proves %s/claude\n' "$r" >"claude-mock/e2e/${r}_test.go"
done
git add -A && git commit -q -m base
BASE=$(git rev-parse HEAD)
export SR_CHECKS_JUDGE_MOCKS='{"file-guard/capability-rigor/tests-prove-as-documented":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'
run() { local status=0; sr-checks run --base "$BASE" --head HEAD >out 2>&1 || status=$?; return 0; }

# a doc re-freeze alone: not judged
git checkout -q -b doc-only "$BASE"
mani 2026-10-09
git add -A && git commit -q -m "re-freeze the doc"
run
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length==0' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "a doc re-freeze alone was judged" >&2; exit 1; }

# the recording c cites changes (the doc re-frozen alongside): judged, and its tests hold up
git checkout -q -b rec-c "$BASE"
: >"$SR_EVENTS_FILE"
mani 2026-10-09
printf '{"e":2}\n' >claude-mock/snapshots/runs/c/samples/20240101-000000/events.jsonl
git add -A && git commit -q -m "record c again"
run
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length==1 and .[0].outcome=="passed"' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "the recording of c was not judged and passed" >&2; exit 1; }

# the recording d cites changes: judged, and the tests proving a capability that cites a section showing nothing of it are refused
git checkout -q -b rec-d "$BASE"
: >"$SR_EVENTS_FILE"
printf '{"e":2}\n' >claude-mock/snapshots/runs/d/samples/20240101-000000/events.jsonl
git add -A && git commit -q -m "record d again"
run
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length==1 and .[0].outcome=="refused" and (.[0].reason | contains("do not drive the mock"))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "the recording of d was not judged and refused" >&2; exit 1; }

# the fix, on top of the refused recording: d cites the section that does show it (a cell change needs no user
# words): judged again, and passed
cap d "d works" s
git add -A && git commit -q -m "d cites the right section"
: >"$SR_EVENTS_FILE"
run
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length==1 and .[0].outcome=="passed"' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "the fixed cell was not judged and passed" >&2; exit 1; }
