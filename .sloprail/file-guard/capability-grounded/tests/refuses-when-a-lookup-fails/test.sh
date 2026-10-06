#!/usr/bin/env bash
set -euo pipefail

# Fail closed: capability-grounded never passes because a lookup failed. Each lookup that decides what is
# checked (which capabilities a change touches: subjects.sh; whether the user's words are required:
# added-or-removed.sh; what the judge is handed: prepare.sh) refuses, with a reason naming what could not be
# read, when it fails. The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's
# rules; only this rule's outcome is asserted. The judge is a mock that decides from its prompt (it refuses a cell citing #undocumented, see the control), so a refusal
# here is the rule's own machinery, never a verdict. The failures are injected with a jq shim, first on PATH for one
# sr-checks run only, that exits non-zero when an argument contains a marker of one lookup's program and
# otherwise runs the real jq. Each scenario is its own commit (a verdict is cached by content, and a stored
# refusal is replayed).
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
mkdir -p bin
# the case's PATH is minimal: the rules' own tool, yq, is linked in from where the machine has it
for p in /opt/homebrew/bin/yq /usr/local/bin/yq /usr/bin/yq; do [ -x "$p" ] && ln -sf "$p" bin/yq && break; done
# a page of the vendor's docs: prepare.sh reads the frozen text, fetched on a miss
printf '#!/bin/sh\nout=""; while [ $# -gt 0 ]; do [ "$1" = -o ] && out="$2"; shift; done\nprintf "# s\\nfrozen text\\n" >"$out"\n' >bin/curl
chmod +x bin/curl
mkdir -p shim
printf '#!/bin/bash\nfor a in "$@"; do case "$a" in *"${SHIM_JQ_FAIL:-@@none@@}"*) exit 5 ;; esac; [ -n "${SHIM_JQ_FAIL_EXACT:-}" ] && [ "$a" = "$SHIM_JQ_FAIL_EXACT" ] && exit 5; done\nexec "%s" "$@"\n' "$(command -v jq)" >shim/jq
chmod +x shim/jq
export PATH="$PWD/bin:$PATH"
SHA="$(printf '# s\nfrozen text\n' | shasum -a 256 | cut -d' ' -f1)"
git init -q .
printf 'bin\nshim\n' >.gitignore
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

n=0
scenario() {   # a new commit on a new branch from BASE: capability c cites another section of its doc (a cell change needs no user words)
  n=$((n + 1)); git checkout -q -b "s$n" "$BASE"; cap "s$n"; git add -A && git commit -q -m "c cites s$n"
}
run_rule() {   # [ENV=VALUE...]
  : > "$SR_EVENTS_FILE"
  env "$@" sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_passed() {   # LABEL
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-grounded")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: capability-grounded did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING
  jq -es --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .rule=="capability-grounded" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: capability-grounded did not refuse with a reason saying '$2' (sr-checks exit $ran)" >&2; exit 1; }
  # the citation requirement (added-or-removed.sh) answers an unreadable change with "the user's words are required", a verdict on what the change needs
  case "$1" in control*|violation|implemented*|new\ code*|*changed-files*|*comparison*) return 0 ;; esac
  jq -es --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .outcome=="refused" and (.reason|contains($s)) and (.reason|contains("could not be evaluated")))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: refused as a verdict, not reported as an error (no verdict to cache)" >&2; exit 1; }
}
inject() {   # MARKER — the lookups whose jq program carries MARKER fail, for this run only
  run_rule "PATH=$PWD/shim:$PATH" "SHIM_JQ_FAIL=$1"
}
inject_exact() {   # PROGRAM — the lookups whose jq program is exactly PROGRAM fail, for this run only
  run_rule "PATH=$PWD/shim:$PATH" "SHIM_JQ_FAIL=" "SHIM_JQ_FAIL_EXACT=$1"
}

# control: nothing injected, the cell change is judged (by the mock) and passes
scenario; run_rule SHIM_JQ_FAIL=
expect_passed "control"

# control: the judge mock decides from its input, so a cell citing a section that shows nothing of the statement is refused
n=$((n + 1)); git checkout -q -b "s$n" "$BASE"; cap undocumented; git add -A && git commit -q -m "c cites #undocumented"
run_rule SHIM_JQ_FAIL=
expect_refused "control: the judge refuses an undocumented section" "the statement claims a part no cited doc or run shows"

# subjects.sh: which capabilities the change touches
scenario; inject 'harnesses:'
expect_refused "the touched capabilities lookup fails" "the capabilities this change touches could not be worked out, so nothing could be judged"

# added-or-removed.sh (the `when` of the user-words requirement): a lookup that fails applies the requirement
scenario; inject 'select(test("^spec/capabilities/"))'
expect_refused "the changed-files listing fails" "the changed capability files could not be listed, so the user's words are required"
scenario; inject '$b.statement != $h.statement'
expect_refused "the comparison fails" "what spec/capabilities/c.yaml changed could not be compared, so the user's words are required"

# prepare.sh: what the judge is handed
scenario; inject_exact '.[]'
expect_refused "the capability list fails" "the capability files could not be listed, so nothing could be prepared for the judge"
scenario; inject '"absent" else "supports"'
expect_refused "the cited docs listing fails" "c: its cited docs could not be listed, so it could not be prepared for the judge"
scenario; inject '(.value.runs // [])[] | [$h, .]'
expect_refused "the cited runs listing fails" "c: its cited runs could not be listed, so it could not be prepared for the judge"
scenario; inject '.doc.statement'
expect_refused "the statement read fails" "c: its statement could not be read, so it could not be prepared for the judge"
scenario; inject 'select(.value == "pending")'
expect_refused "the pending cells read fails" "c: its pending cells could not be read, so it could not be prepared for the judge"
scenario; inject 'removed: false'
expect_refused "the context assembly fails" "c: its context could not be assembled, so it could not be prepared for the judge"
