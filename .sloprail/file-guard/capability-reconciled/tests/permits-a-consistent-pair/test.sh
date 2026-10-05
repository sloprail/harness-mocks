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

# a violation outside the change (w: supported, no adapter) is not re-judged by a change to x alone
git checkout -q -b elsewhere "$BASE"
git checkout -q "$BASE"; git checkout -q -b base2
cap w supported; c "w, no adapter"
BASE2=$(git rev-parse HEAD)
marker claude-mock/internal/x.go x/claude; printf '// edited\n' >> claude-mock/internal/x.go; c "x adapter edited"
expect_passed "$BASE2" "an untouched pair's violation was re-judged"

# a new capability and its adapter, together
git checkout -q -b added "$BASE"
cap z supported; marker claude-mock/internal/z.go z/claude; c "z and its adapter"
expect_passed "$BASE" "a consistent new pair"

# a cell made unsupported with its adapter removed, in one commit
git checkout -q -b retired "$BASE"
cap x unsupported; git rm -q claude-mock/internal/x.go; c "x retired"
expect_passed "$BASE" "a cell and its adapter retired together"

# a cell made pending with its adapter removed
git checkout -q -b parked "$BASE"
cap x pending; git rm -q claude-mock/internal/x.go; c "x pending"
expect_passed "$BASE" "a cell parked as pending with its adapter removed"
