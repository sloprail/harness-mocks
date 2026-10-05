#!/usr/bin/env bash
set -euo pipefail

# Fail closed, part two: each lookup of capability-grounded's prepare.sh (what the judge is handed) refuses,
# with a reason naming what could not be read, when it fails: the changed-files listing, a capability's id,
# the harness narrowing, the doc assembly, the cited runs, the unsupported cells, the deleted-files listing,
# the subject count and the final output (and, through cells.sh, the changed capability files). The other
# lookups are refuses-when-a-lookup-fails'. The CI path, no agent turn: `sr-checks run` judges committed
# ranges; the judge is a mock that always passes, so a refusal here is the rule's own machinery, never a
# verdict. The failures are injected with a jq shim, first on PATH for one sr-checks run only, that exits
# non-zero when an argument contains a marker (SHIM_JQ_FAIL, SHIM_JQ_FAIL2) or equals SHIM_JQ_FAIL_EXACT, and
# otherwise runs the real jq. Each marker is one a single lookup carries. Each scenario is its own commit
# (a verdict is cached by content, and a stored refusal is replayed).
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
mkdir -p bin
# the case's PATH is minimal: the rules' own tool, yq, is linked in from where the machine has it
for p in /opt/homebrew/bin/yq /usr/local/bin/yq /usr/bin/yq; do [ -x "$p" ] && ln -sf "$p" bin/yq && break; done
# a page of the vendor's docs: prepare.sh reads the frozen text, fetched on a miss
printf '#!/bin/sh\nout=""; while [ $# -gt 0 ]; do [ "$1" = -o ] && out="$2"; shift; done\nprintf "# s\\nfrozen text\\n" >"$out"\n' >bin/curl
chmod +x bin/curl
mkdir -p shim
printf '#!/bin/bash\nhit=""\nfor a in "$@"; do\n  case "$a" in *"${SHIM_JQ_FAIL:-@@none@@}"*) hit=1 ;; esac\n  case "$a" in *"${SHIM_JQ_FAIL2:-@@none@@}"*) hit=1 ;; esac\n  [ -n "${SHIM_JQ_FAIL_EXACT:-}" ] && [ "$a" = "$SHIM_JQ_FAIL_EXACT" ] && hit=1\ndone\n[ -n "$hit" ] && exit 5\nexec "%s" "$@"\n' "$(command -v jq)" >shim/jq
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
# c has two providers, so a change to the claude cell alone narrows the harnesses in question; codex has no
# mock in the tree and is never read. d is a second capability, there to be deleted.
cap() { printf 'statement: c works\nproviders:\n  claude:\n    docs: [https://d.example/p#%s]\n    runs: [claude-mock/snapshots/runs/c]\n  codex:\n    docs: [https://d.example/p#s]\n    runs: [codex-mock/snapshots/runs/c]\n' "$1" >spec/capabilities/c.yaml; }
cap s
printf 'statement: d works\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/c]\n' >spec/capabilities/d.yaml
git add -A && git commit -q -m base
BASE=$(git rev-parse HEAD)
export SR_CHECKS_JUDGE_MOCKS='{"file-guard/capability-grounded/docs-support-statement":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'

n=0
scenario() {   # a new commit on a new branch from BASE: c's claude cell cites another section of its doc (a cell change needs no user words)
  n=$((n + 1)); git checkout -q -b "s$n" "$BASE"; cap "s$n"; git add -A && git commit -q -m "c cites s$n"
}
scenario_delete() {   # a new commit on a new branch from BASE: capability d is deleted
  n=$((n + 1)); git checkout -q -b "s$n" "$BASE"; git rm -q spec/capabilities/d.yaml; git commit -q -m "d is gone"
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
}
inject() {   # MARKER [MARKER2] — the lookups whose jq program carries a marker fail, for this run only
  run_rule "PATH=$PWD/shim:$PATH" "SHIM_JQ_FAIL=$1" "SHIM_JQ_FAIL2=${2:-}"
}
inject_exact() {   # PROGRAM — the lookups whose jq program is exactly PROGRAM fail, for this run only
  run_rule "PATH=$PWD/shim:$PATH" "SHIM_JQ_FAIL=" "SHIM_JQ_FAIL_EXACT=$1"
}

# control: nothing injected, the cell change is judged (by the mock) and passes; so is a deletion
scenario; run_rule SHIM_JQ_FAIL=
expect_passed "control: a cell change"

# the changed files listing (the whole changeset's paths)
scenario; inject_exact '.changeset.files[].path'
expect_refused "the changed files listing fails" "the changed files could not be listed, so nothing could be prepared for the judge"
# and cells.sh's, when its one-pass table cannot be built (the final jq of the fast path fails too) and the file-by-file list cannot be either
scenario; inject '. as $f | (if $f.st' '.changeset.files[].path | select(test('
expect_refused "the capability files listing fails" "the changed capability files could not be listed, so what they touch could not be worked out"

# a capability's id (the exact program `.id`)
scenario; inject_exact '.id'
expect_refused "the id read fails" "a capability's id could not be read, so it could not be prepared for the judge"

# the harnesses in question narrowed (c's codex cell is unchanged, so only claude is in question)
scenario; inject 'with_entries(select(.key as $k'
expect_refused "the harness narrowing fails" "c: its harnesses in question could not be narrowed, so it could not be prepared for the judge"

# the docs assembled
scenario; inject '{harness: $h, kind: $k, ref: $r, path: $p'
expect_refused "the docs assembly fails" "c: the cited doc https://d.example/p#s"

# the cited runs read, and the unsupported cells read
scenario; inject '.value.runs[]? | {harness: $h, kind: $k, path: .}'
expect_refused "the runs read fails" "c: its cited runs could not be read, so it could not be prepared for the judge"
scenario; inject '{harness: .key, reason: .value.reason}'
expect_refused "the unsupported cells read fails" "c: its unsupported cells could not be read, so it could not be prepared for the judge"

# the deleted capability files listed. A deletion needs the user's words (a citation that resolves in a
# session), which `sr-checks run` cannot have here, so prepare.sh is run as the engine runs it: with the
# payload `sr-checks changeset` prints for the subject, and the rule's own environment.
scenario_delete
sr-checks changeset --rule capability-grounded --base "$BASE" --head HEAD | jq -c '.subjects[0].payload' >delete.payload
prepare() {   # [ENV=VALUE...] — prepare.sh over the deletion's payload; a refusal is {"reason"} on stdout
  (cd .sloprail/file-guard/capability-grounded && env "$@" SR_TREE="$OLDPWD" SR_GUARDRAIL_DIR="$PWD" bash ./prepare.sh <"$OLDPWD/delete.payload")
}
expect_prepare_refused() {   # LABEL SUBSTRING
  printf '%s' "$out" | jq -e --arg s "$2" '.reason | contains($s)' >/dev/null ||
    { echo "$1: prepare.sh did not refuse with a reason saying '$2' : $out" >&2; exit 1; }
}
out="$(prepare SHIM_JQ_FAIL=)"
printf '%s' "$out" | jq -e '.additionalContext.subjects[0] | .id == "d" and .removed == true' >/dev/null ||
  { echo "control: a deletion: prepare.sh did not hand the judge the removed capability: $out" >&2; exit 1; }
out="$(prepare "PATH=$PWD/shim:$PATH" 'SHIM_JQ_FAIL=startswith("spec/capabilities/"))) | .path')" &&
  { echo "the deleted files listing fails: prepare.sh exited 0: $out" >&2; exit 1; }
expect_prepare_refused "the deleted files listing fails" "the deleted capability files could not be listed, so nothing could be prepared for the judge"
out="$(prepare "PATH=$PWD/shim:$PATH" 'SHIM_JQ_FAIL=removed: true')" &&
  { echo "the deleted capability assembly fails: prepare.sh exited 0: $out" >&2; exit 1; }
expect_prepare_refused "the deleted capability assembly fails" "spec/capabilities/d.yaml: the deleted capability could not be listed, so it could not be prepared for the judge"

# a long changed-files list (over 64 KB, the pipe buffer) must not lose a capability: the match on the list
# is no `printf | grep -q`, which dies of SIGPIPE (a failed match under pipefail) once grep quits early
scenario
sr-checks changeset --rule capability-grounded --base "$BASE" --head HEAD | jq -c '.subjects[0].payload' >cell.payload
jq -c '.changeset.files += [range(0; 2500) | {path: ("pad/" + ("x" * 40) + (. | tostring)), status: "A"}]' cell.payload >big.payload
[ "$(jq -r '[.changeset.files[].path] | join("\n") | length' big.payload)" -gt 65536 ] || { echo "the padded changed-files list is not over 64 KB" >&2; exit 1; }
if out="$(cd .sloprail/file-guard/capability-grounded && SR_TREE="$OLDPWD" SR_GUARDRAIL_DIR="$PWD" bash ./prepare.sh <"$OLDPWD/big.payload")"; then big_rc=0; else big_rc=$?; fi
[ "$big_rc" -eq 0 ] && printf '%s' "$out" | jq -e '.additionalContext.subjects[0].id == "c"' >/dev/null ||
  { echo "a changed-files list over 64 KB: prepare.sh dropped the changed capability (exit $big_rc): $out" >&2; exit 1; }

# the count of the subjects, and the output
scenario; inject_exact 'length'
expect_refused "the subject count fails" "the prepared subjects could not be counted, so nothing could be handed to the judge"
scenario; inject '{additionalContext: {subjects: .}}'
expect_refused "the output write fails" "the prepared subjects could not be written, so nothing could be handed to the judge"
