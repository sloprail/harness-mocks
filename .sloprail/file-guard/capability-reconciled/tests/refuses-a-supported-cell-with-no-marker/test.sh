#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only this
# rule's outcome is asserted.
# the rules read the specs with yq, which the case's PATH (jq, git, bash, the sloprail binaries) does not carry
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
git init -q .
c() { git add -A && git -c user.name=t -c user.email=t@t commit -q -m "$1"; }
# cap ID KIND — a capability whose claude cell is supported, unsupported or pending
cap() {
  mkdir -p spec/capabilities
  case "$2" in
    supported) cell=$'  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/r1]' ;;
    unsupported) cell=$'  claude:\n    supported: false\n    reason: the harness lacks it\n    docs: [https://d.example/p#s]' ;;
    pending) cell=$'  claude: pending' ;;
  esac
  printf 'statement: %s works\nproviders:\n%s\n' "$1" "$cell" > "spec/capabilities/$1.yaml"
}
# marker FILE FQN — adapter code carrying sr:provides
marker() { mkdir -p claude-mock/internal; printf 'package internal\n\n// sr:provides %s\nfunc adapter() {}\n' "$2" > "$1"; }
# run_rule BASE — judge BASE..HEAD with the project's rules; this rule's FileGuardChecked events land in $SR_EVENTS_FILE
run_rule() {
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$1" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_passed() {   # BASE LABEL
  run_rule "$1"
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-reconciled")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$2: capability-reconciled did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # BASE LABEL SUBSTRING... : refused, and the reason says each substring
  local base="$1" label="$2" s; shift 2
  run_rule "$base"
  for s in "$@"; do
    jq -es --arg s "$s" 'any(.[]; .kind=="FileGuardChecked" and .rule=="capability-reconciled" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
      { jq -c . "$SR_EVENTS_FILE" >&2; echo "$label: capability-reconciled did not refuse with a reason saying '$s' (sr-checks exit $ran)" >&2; exit 1; }
  done
}

mkdir -p claude-mock/snapshots/runs/r1 codex-mock
echo "version: 1" > claude-mock/snapshots/runs/r1/run.yaml
# the base: capability x, supported, with its adapter
cap x supported; marker claude-mock/internal/x.go x/claude; c base
BASE=$(git rev-parse HEAD)

# the adapter is deleted, the cell stays supported: refused, naming the cell
git checkout -q -b deleted "$BASE"
git rm -q claude-mock/internal/x.go; c "x adapter deleted"
expect_refused "$BASE" "a supported cell whose adapter was deleted" "spec/capabilities/x.yaml" "x/claude" "carries // sr:provides x/claude"
# recovery: the cell follows (made unsupported), and the same range passes
cap x unsupported; c "x becomes unsupported"
expect_passed "$BASE" "the cell followed the deleted adapter"

# a new supported cell with no adapter: refused
git checkout -q -b added "$BASE"
cap z supported; c "z, no adapter"
expect_refused "$BASE" "a new supported cell with no adapter" "spec/capabilities/z.yaml" "z/claude"
# recovery: the adapter arrives
marker claude-mock/internal/z.go z/claude; c "z adapter"
expect_passed "$BASE" "the adapter arrived"

# a supported cell that cites a missing recording
git checkout -q -b norun "$BASE"
printf 'statement: x works\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/gone]\n' > spec/capabilities/x.yaml; c "x cites a missing run"
expect_refused "$BASE" "a cell citing a missing recording" "'x' × 'claude'" "claude-mock/snapshots/runs/gone"

# a supported cell that cites no recording at all (an empty runs list)
git checkout -q -b emptyruns "$BASE"
printf 'statement: x works\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: []\n' > spec/capabilities/x.yaml; c "x cites no run"
expect_refused "$BASE" "a cell citing no recording" "'x' × 'claude'" "cites no recorded run"

# a cited recording deleted while the cell stays: refused, naming the run
git checkout -q -b rundeleted "$BASE"
git rm -q -r claude-mock/snapshots/runs/r1; c "r1 deleted"
expect_refused "$BASE" "a cited recording deleted" "'x' × 'claude'" "claude-mock/snapshots/runs/r1"
