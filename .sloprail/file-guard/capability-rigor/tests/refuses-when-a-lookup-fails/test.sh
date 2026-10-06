#!/usr/bin/env bash
set -euo pipefail

# Fail closed: capability-rigor never passes because a lookup failed. Each lookup that decides what is checked
# or what the judge is handed refuses, with a reason naming what could not be read: which pairs a change touches
# and which capabilities (subjects.sh), a cell's cited docs and runs (inputs-ready.sh, the check ahead of the
# judge), and the judge's context (prepare.sh). The CI path, no agent turn: `sr-checks run` judges committed
# ranges with the project's rules; only this rule's outcome is asserted. The judge is a mock that always passes,
# so a refusal here is the rule's own machinery, never a verdict. The failures are injected with a jq shim,
# first on PATH for one sr-checks run only, that exits non-zero when an argument contains a marker of one
# lookup's program and otherwise runs the real jq. Each scenario is its own commit (a verdict is cached by
# content, and a stored refusal is replayed).
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
mkdir -p bin
# the case's PATH is minimal: the rules' own tool, yq, is linked in from where the machine has it
for p in /opt/homebrew/bin/yq /usr/local/bin/yq /usr/bin/yq; do [ -x "$p" ] && ln -sf "$p" bin/yq && break; done
# a page of the vendor's docs: prepare.sh reads the frozen text, fetched on a miss
printf '#!/bin/sh\nout=""; while [ $# -gt 0 ]; do [ "$1" = -o ] && out="$2"; shift; done\nprintf "# s\\nfrozen text\\n" >"$out"\n' >bin/curl
chmod +x bin/curl
mkdir -p shim
printf '#!/bin/bash\nfor a in "$@"; do case "$a" in *"${SHIM_JQ_FAIL:-@@none@@}"*) exit 5 ;; esac; done\nexec "%s" "$@"\n' "$(command -v jq)" >shim/jq
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
  case "$1" in control*|violation|implemented*|new\ code*) return 0 ;; esac
  jq -es --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .outcome=="refused" and (.reason|contains($s)) and (.reason|contains("could not be evaluated")))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: refused as a verdict, not reported as an error (no verdict to cache)" >&2; exit 1; }
}
inject() {   # MARKER — the lookups whose jq program carries MARKER fail, for this run only
  run_rule "PATH=$PWD/shim:$PATH" "SHIM_JQ_FAIL=$1"
}

# control: nothing injected, the statement change is judged (by the mock) and passes
scenario; run_rule SHIM_JQ_FAIL=
expect_passed "control"

# subjects.sh: which pairs and capabilities the change touches
scenario; inject '$want == ""'
expect_refused "the pairs lookup fails" "the touched capability pairs could not be worked out, so nothing could be judged"
scenario; inject '"pair:"'
expect_refused "the subjects assembly fails" "the capabilities this change touches could not be worked out, so nothing could be judged"

# inputs-ready.sh: a cell's cited docs and runs
scenario; inject '(.docs // [])'
expect_refused "the cited docs listing fails (inputs)" "c/claude: its cited docs could not be listed, so they could not be checked"
scenario; inject '(.runs // [])'
expect_refused "the cited runs listing fails (inputs)" "c/claude: its cited runs could not be listed, so they could not be checked"

# prepare.sh: what the judge is handed (inputs-ready passes, so these lookups are reached)
scenario; inject '.doc.statement'
expect_refused "the statement read fails" "c/claude: its statement could not be read, so it could not be prepared for the judge"
scenario; inject 'context: {harness'
expect_refused "the context assembly fails" "c/claude: its context could not be assembled, so it could not be prepared for the judge"
