#!/usr/bin/env bash
set -euo pipefail

# A regression case for inputs-ready.sh's existing refusals and their recovery (an unproven pair, an
# unsealed run, an unfrozen doc, each refused and then passing once restored). It is not a fail-on-old
# case: it may pass on the script as it was before the lookups were made to refuse (#204); those failures
# are refuses-when-a-lookup-fails' and refuses-when-a-prepare-lookup-fails'.

# capability-rigor asks no model until the judge's inputs exist (inputs-ready.sh): every doc a touched pair
# cites is frozen in the MANIFEST, every cited run has a sealed sample, and a test carries
# // sr:proves <id>/<h>. Each missing input is refused, with a reason naming it and its owner; once it is
# back the same range passes. The CI path, no agent turn: `sr-checks run` judges committed ranges with the
# project's rules; only this rule's outcome is asserted. The judge is a mock that always passes, so a
# refusal here is the rule's own machinery, never a verdict.
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
mkdir -p spec/capabilities claude-mock/snapshots/runs/c/samples/20240101-000000 claude-mock/e2e
manifest() { printf 'pin: "1"\ndocs:\n  https://d.example/p:\n    sha256: %s\n    fetched: "2026-10-01"\n' "$SHA" >claude-mock/snapshots/MANIFEST.yaml; }
manifest
printf 'version: 1\n' >claude-mock/snapshots/runs/c/run.yaml
printf '{"e":1}\n' >claude-mock/snapshots/runs/c/samples/20240101-000000/events.jsonl
SEAL=claude-mock/snapshots/runs/c/samples/20240101-000000/SEAL
(cd claude-mock/snapshots/runs/c/samples/20240101-000000 && shasum -a 256 ./events.jsonl >SEAL)
proof() { mkdir -p claude-mock/e2e; printf 'package e2e\n\n// sr:proves c/claude\nfunc TestC() {}\n' >claude-mock/e2e/c_test.go; mkdir -p claude-mock/e2e/018_replay; printf 'package e2e\n\nvar notReplaying = map[string]string{\n}\n' >claude-mock/e2e/018_replay/replay_allowlist_test.go; }
proof
cap() { printf 'statement: %s\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/c]\n' "$1" >spec/capabilities/c.yaml; }
cap "c works"
git add -A && git commit -q -m base
BASE=$(git rev-parse HEAD)
export SR_CHECKS_JUDGE_MOCKS='{"file-guard/capability-rigor/tests-prove-as-documented":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'

run_rule() {
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_passed() {   # LABEL
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: capability-rigor did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING
  jq -es --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .rule=="capability-rigor" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: capability-rigor did not refuse with a reason saying '$2' (sr-checks exit $ran)" >&2; exit 1; }
}
# each scenario: c's statement changes (which touches its pair) while one input is missing; the next commit restores it
scenario() {   # NAME STATEMENT
  git checkout -q -b "$1" "$BASE"; cap "$2"
}

scenario no-proof "c works, differently (no proof)"; git rm -q claude-mock/e2e/c_test.go; git add -A && git commit -q -m "c changes, its proving test is gone"
run_rule
expect_refused "a pair no test proves" "c/claude: no test carries // sr:proves c/claude"
proof; git add -A && git commit -q -m "the proving test is back"
run_rule
expect_passed "the proving test is back"

scenario no-seal "c works, differently (no seal)"; git rm -q "$SEAL"; git add -A && git commit -q -m "c changes, its sample is unsealed"
run_rule
expect_refused "an unsealed run" "c/claude: claude-mock/snapshots/runs/c has no sealed sample"
(cd claude-mock/snapshots/runs/c/samples/20240101-000000 && shasum -a 256 ./events.jsonl >SEAL); git add -A && git commit -q -m "the sample is sealed again"
run_rule
expect_passed "the sample is sealed again"

scenario no-freeze "c works, differently (no freeze)"; printf 'pin: "1"\ndocs: {}\n' >claude-mock/snapshots/MANIFEST.yaml; git add -A && git commit -q -m "c changes, its doc is not frozen"
run_rule
expect_refused "an unfrozen doc" "c/claude: claude-mock/snapshots/MANIFEST.yaml does not freeze https://d.example/p"
manifest; git add -A && git commit -q -m "the doc is frozen again"
run_rule
expect_passed "the doc is frozen again"
