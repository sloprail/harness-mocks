#!/usr/bin/env bash
set -euo pipefail
# Docs follow recordings: snapshots-current reads no live page (no network), so a doc that changed upstream
# since it was frozen is no refusal, while a cited doc the MANIFEST does not freeze at all still is.
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
sha() { printf '%s' "$1" | shasum -a 256 | cut -d' ' -f1; }
mkdir -p bin
# the case's PATH is minimal: the rules' own tool, yq, is linked in from where the machine has it
for p in /opt/homebrew/bin/yq /usr/local/bin/yq /usr/bin/yq ; do [ -x "$p" ] && ln -sf "$p" bin/yq && break; done
# any fetch of a page is logged and fails: the check must stay offline
printf '#!/bin/sh\necho "fetched $*" >>"%s/fetched"\nexit 1\n' "$PWD" >bin/curl
chmod +x bin/curl
export PATH="$PWD/bin:$PATH"
git init -q .
printf 'bin\nout\nfetched\n' >.gitignore
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
printf 'disabled:\n  - sloprail/gate/ci-verify-required\n  - sloprail/file-guard/rule-tests-pass\n' >.sloprail/config.yaml
git add -A && git commit -q -m base
BASE=$(git rev-parse HEAD)
mkdir -p claude-mock/snapshots/runs/r/samples/20240101-000000 spec/capabilities
# the page was frozen at this hash; upstream it has changed since (the fake curl cannot even serve it)
printf 'pin: "1"\ndocs:\n  https://d.example/p:\n    sha256: %s\n    fetched: "2026-10-01"\n' "$(sha $'# s\nfrozen text\n')" >claude-mock/snapshots/MANIFEST.yaml
printf '#!/bin/sh\n' >claude-mock/snapshots/capture.sh
printf 'version: 1\n' >claude-mock/snapshots/runs/r/run.yaml
printf '{"e":1}\n' >claude-mock/snapshots/runs/r/samples/20240101-000000/events.jsonl
(cd claude-mock/snapshots/runs/r/samples/20240101-000000 && find . -type f ! -name 'SEAL*' | LC_ALL=C sort | xargs shasum -a 256 >SEAL)
printf 'statement: c works\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/r]\n' >spec/capabilities/c.yaml
git add -A && git commit -q -m "a frozen doc, a recording, a capability citing both"
run() { local status=0; sr-checks run --base "$BASE" --head HEAD >out 2>&1 || status=$?; return 0; }

# permitted: the live page is not looked at
: >"$SR_EVENTS_FILE"
run
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="snapshots-current")] | length==1 and .[0].outcome=="passed"' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "a doc frozen earlier made snapshots-current refuse" >&2; exit 1; }
[ ! -e fetched ] || { cat fetched >&2; echo "snapshots-current fetched a page" >&2; exit 1; }

# refused: the capability cites a page the MANIFEST does not freeze (the nearest neighbour: one URL off)
printf 'statement: c works\nproviders:\n  claude:\n    docs: [https://d.example/unfrozen#s]\n    runs: [claude-mock/snapshots/runs/r]\n' >spec/capabilities/c.yaml
git add -A && git commit -q -m "cite an unfrozen page"
: >"$SR_EVENTS_FILE"
run
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="snapshots-current")] | length==1 and .[0].outcome=="refused" and (.[0].reason | contains("which no snapshot in claude-mock/snapshots/MANIFEST.yaml freezes"))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "an unfrozen cited page was not refused" >&2; exit 1; }

# recovery: the capability cites the frozen page again, and the same range passes
printf 'statement: c works\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/r]\n' >spec/capabilities/c.yaml
git add -A && git commit -q -m "cite the frozen page again"
: >"$SR_EVENTS_FILE"
run
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="snapshots-current")] | length==1 and .[0].outcome=="passed"' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "citing the frozen page again was not passed by snapshots-current" >&2; exit 1; }
