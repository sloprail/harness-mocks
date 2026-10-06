#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn. Adding or deleting a <h>-mock/ directory changes the harness set, and then EVERY capability
# is judged for its cells, touched by the change or not: a cell for every harness mock, none for a harness with no mock.
# Capabilities x and y are proven for claude and codex. A change that only adds gemini-mock/ touches neither of them, and
# is refused for both; one that only deletes a mock whose cells remain is refused too. A change to claude's recording alone
# still passes.
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
git init -q .
c() { git add -A && git -c user.name=t -c user.email=t@t commit -q -m "$1"; }
run_rule() {
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$1" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_passed() {   # BASE LABEL
  run_rule "$1"
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-covered")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$2: capability-covered did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # BASE LABEL SUBSTRING... : refused, and the one reason says each
  local base="$1" label="$2" s; shift 2
  run_rule "$base"
  for s in "$@"; do
    jq -es --arg s "$s" 'any(.[]; .kind=="FileGuardChecked" and .rule=="capability-covered" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
      { jq -c . "$SR_EVENTS_FILE" >&2; echo "$label: capability-covered did not refuse with a reason saying '$s' (sr-checks exit $ran)" >&2; exit 1; }
  done
}

mkdir -p claude-mock/snapshots/runs/r1 codex-mock/snapshots/runs/r1 claude-mock/e2e codex-mock/e2e internal/core spec/capabilities
echo "version: 1" > claude-mock/snapshots/runs/r1/run.yaml
echo "version: 1" > codex-mock/snapshots/runs/r1/run.yaml
for id in x y; do
  printf 'package core\n\n// sr:capability %s\nfunc %s() {}\n' "$id" "$(echo $id | tr a-z A-Z)" > internal/core/$id.go
  for h in claude codex; do printf 'package e2e\n\n// sr:proves %s/%s\nfunc Test%s() {}\n' "$id" "$h" "$(echo $id | tr a-z A-Z)" > $h-mock/e2e/${id}_test.go; done
done
cells() {   # [gemini]: with a gemini cell too
  for id in x y; do
    { printf 'statement: %s works\nproviders:\n' "$id"
      for h in claude codex; do printf '  %s:\n    docs: [https://d.example/p#s]\n    runs: [%s-mock/snapshots/runs/r1]\n' "$h" "$h"; done
      [ "${1:-}" = gemini ] && printf '  gemini:\n    docs: [https://d.example/p#s]\n    runs: [gemini-mock/snapshots/runs/r1]\n'
      true; } > spec/capabilities/$id.yaml
  done
}
cells; c base
BASE=$(git rev-parse HEAD)

# boundary: a change to claude's recording alone does not change the harness set, and passes
git checkout -q -b rec-claude "$BASE"
echo "version: 2" > claude-mock/snapshots/runs/r1/run.yaml; c "claude is recorded again"
expect_passed "$BASE" "a recording change, the harness set unchanged"

# a harness mock is added and no capability gets its cell: both capabilities are refused though neither is touched
git checkout -q -b add-gemini "$BASE"
mkdir -p gemini-mock/snapshots/runs/r1; echo "version: 1" > gemini-mock/snapshots/runs/r1/run.yaml; c "gemini-mock is added"
expect_refused "$BASE" "a new mock dir with no cell" "capability 'x' has no cell for 'gemini'" "capability 'y' has no cell for 'gemini'"

# recovery: both capabilities get their gemini cell and a proving test
mkdir -p gemini-mock/e2e; cells gemini
for id in x y; do printf 'package e2e\n\n// sr:proves %s/gemini\nfunc Test%s() {}\n' "$id" "$(echo $id | tr a-z A-Z)" > gemini-mock/e2e/${id}_test.go; done
c "gemini cells and proofs"
expect_passed "$BASE" "the new mock dir has its cells"
WITH=$(git rev-parse HEAD)

# a harness mock is deleted and its cells remain: refused for both capabilities though neither is touched
git checkout -q -b del-gemini "$WITH"
git rm -rq gemini-mock; c "gemini-mock is deleted"
expect_refused "$WITH" "a deleted mock dir whose cells remain" "capability 'x' has a cell for 'gemini', but there is no gemini-mock/" "capability 'y' has a cell for 'gemini', but there is no gemini-mock/"

# recovery: the cells go with the mock
cells; c "the gemini cells are removed"
expect_passed "$WITH" "the deleted mock dir has no cells left"
