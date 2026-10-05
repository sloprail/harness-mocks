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
