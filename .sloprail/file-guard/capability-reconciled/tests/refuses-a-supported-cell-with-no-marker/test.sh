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
# verdict BASE — this rule's outcome over BASE..HEAD: its refusal reason, or "passed"
verdict() {
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$1" --head HEAD >/dev/null 2>&1 || true
  jq -rs '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-reconciled")] | if length == 0 then "did not run" elif all(.[]; .outcome=="passed") then "passed" else map(.reason // .outcome) | join(" | ") end' "$SR_EVENTS_FILE"
}
expect_passed() { v="$(verdict "$1")"; [ "$v" = passed ] || { echo "$2: expected passed, got: $v" >&2; exit 1; }; }
expect_refused() {   # BASE LABEL SUBSTRING...
  v="$(verdict "$1")"; l="$2"; shift 2
  [ "$v" != passed ] && [ "$v" != "did not run" ] || { echo "$l: expected a refusal, got: $v" >&2; exit 1; }
  for s in "$@"; do case "$v" in *"$s"*) ;; *) echo "$l: the refusal does not say '$s': $v" >&2; exit 1 ;; esac; done
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

# a supported cell that cites no recording, and one that cites a missing one
git checkout -q -b norun "$BASE"
printf 'statement: x works\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/gone]\n' > spec/capabilities/x.yaml; c "x cites a missing run"
expect_refused "$BASE" "a cell citing a missing recording" "'x' × 'claude'" "claude-mock/snapshots/runs/gone"
