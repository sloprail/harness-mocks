#!/usr/bin/env bash
set -euo pipefail

# A deviation of kind harness-lacks is waived from the user's words because a cited doc or recording shows it
# (adr/capability-grounding), so a cell with one must cite a doc or a run: harness-lacks-cited.sh refuses a cell
# that cites neither, and the same change passes once it cites a doc and a run. The CI path, no agent turn:
# `sr-checks run` judges committed ranges with the project's rules; only this rule's outcome is asserted.
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
mkdir -p bin
# the case's PATH is minimal: the rules' own tool, yq, is linked in from where the machine has it
for p in /opt/homebrew/bin/yq /usr/local/bin/yq /usr/bin/yq; do [ -x "$p" ] && ln -sf "$p" bin/yq && break; done
# a page of the vendor's docs: prepare.sh reads the frozen text, fetched on a miss
printf '#!/bin/sh\nout=""; while [ $# -gt 0 ]; do [ "$1" = -o ] && out="$2"; shift; done\nprintf "# s\\nfrozen text\\n" >"$out"\n' >bin/curl
chmod +x bin/curl
export PATH="$PWD/bin:$PATH"
SHA="$(printf '# s\nfrozen text\n' | shasum -a 256 | cut -d' ' -f1)"
git init -q .
printf 'bin\n' >.gitignore
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
printf 'disabled:\n  - sloprail/gate/ci-verify-required\n  - sloprail/file-guard/rule-tests-pass\n' >.sloprail/config.yaml
mkdir -p spec/capabilities claude-mock/snapshots/runs/c/samples/20240101-000000
printf 'pin: "1"\ndocs:\n  https://d.example/p:\n    sha256: %s\n    fetched: "2026-10-01"\n' "$SHA" >claude-mock/snapshots/MANIFEST.yaml
printf 'version: 1\n' >claude-mock/snapshots/runs/c/run.yaml
printf '{"e":1}\n' >claude-mock/snapshots/runs/c/samples/20240101-000000/events.jsonl
cap() { printf 'statement: c works\nproviders:\n  claude:\n    docs: [https://d.example/p#%s]\n    runs: [claude-mock/snapshots/runs/c]\n' "$1" >spec/capabilities/c.yaml; }
cap s
git add -A && git commit -q -m base
BASE=$(git rev-parse HEAD)
export SR_CHECKS_JUDGE_MOCKS='{"file-guard/capability-grounded/docs-support-statement":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'

run_rule() {
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
outcome() {   # the outcome capability-grounded reached over the range
  jq -rs '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-grounded")] | if length == 0 then "none" elif all(.[]; .outcome=="passed") then "passed" else "refused" end' "$SR_EVENTS_FILE"
}
# harness-lacks-cited.sh judged on the payload the engine hands it (the citation requirement of added-or-removed.sh
# comes before it in a real run, and needs a user's words a sandbox has none of)
check() {   # prints the script's stdout, sets rc
  local payload
  payload="$(sr-checks changeset --rule capability-grounded --base "$BASE" --head HEAD | jq -c '.subjects[0].payload')"
  out="$(cd .sloprail/file-guard/capability-grounded && SR_TREE="$PWD/../../.." SR_GUARDRAIL_DIR="$PWD" bash ./harness-lacks-cited.sh <<<"$payload")" && rc=0 || rc=$?
}
# lacks DOCS RUNS — capability c with a harness-lacks deviation on claude, citing DOCS and RUNS (YAML flow lists)
lacks() { printf 'statement: c works\nproviders:\n  claude:\n    docs: %s\n    runs: %s\n    deviations:\n      - adr: harness-gap\n        kind: harness-lacks\n        statement: the harness has no such command\n' "$1" "$2" >spec/capabilities/c.yaml; }

git checkout -q -b lacks "$BASE"
lacks '[]' '[]'
git add -A && git commit -q -m "a harness-lacks deviation citing neither a doc nor a run"
run_rule
[ "$(outcome)" = refused ] || { jq -c . "$SR_EVENTS_FILE" >&2; echo "capability-grounded did not refuse a harness-lacks deviation citing nothing" >&2; exit 1; }
check
[ "$rc" -eq 1 ] && printf '%s' "$out" | jq -e '(.reason | contains("have a kind: harness-lacks deviation and cites no doc or run")) and (.error | not)' >/dev/null ||
  { echo "a harness-lacks deviation citing nothing was not refused as a verdict (exit $rc): $out" >&2; exit 1; }

# recovery: it cites a doc section and a recorded run, and the same range from the same base passes
lacks '[https://d.example/p#s]' '[claude-mock/snapshots/runs/c]'
git add -A && git commit -q -m "the deviation cites a doc and a run"
run_rule
[ "$(outcome)" = passed ] || { jq -c . "$SR_EVENTS_FILE" >&2; echo "capability-grounded did not pass a harness-lacks deviation citing a doc and a run (sr-checks exit $ran)" >&2; exit 1; }
check
[ "$rc" -eq 0 ] || { echo "a harness-lacks deviation citing a doc and a run was refused (exit $rc): $out" >&2; exit 1; }
