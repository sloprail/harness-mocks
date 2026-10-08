#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# capability-covered's outcome is asserted. A capability is judged per harness: one harness's missing tests never refuse a
# change that touches only another harness. Capability x is proven for claude and has a supported cell for codex with no
# test marked sr:proves x/codex: a change to claude's recording passes, a change to codex's recording or to the shared
# statement is refused for codex.
# the rules read the specs with yq, which the case's PATH (jq, git, bash, the sloprail binaries) does not carry
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
expect_refused() {   # BASE LABEL SUBSTRING : refused, and the reason says it
  run_rule "$1"
  jq -es --arg s "$3" 'any(.[]; .kind=="FileGuardChecked" and .rule=="capability-covered" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$2: capability-covered did not refuse with a reason saying '$3' (sr-checks exit $ran)" >&2; exit 1; }
}

# two harnesses; capability x implemented once in internal/; claude proves it, codex has a supported cell and no proof
mkdir -p claude-mock/snapshots/runs/r1 codex-mock/snapshots/runs/r1 claude-mock/e2e internal/core spec/capabilities
echo "version: 1" > claude-mock/snapshots/runs/r1/run.yaml
echo "version: 1" > codex-mock/snapshots/runs/r1/run.yaml
printf 'package core\n\n// sr:capability x\nfunc X() {}\n' > internal/core/x.go
printf 'package e2e\n\n// sr:proves x/claude\nfunc TestX() {}\n' > claude-mock/e2e/x_test.go
statement() { printf 'statement: %s\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/r1]\n  codex:\n    docs: [https://d.example/p#s]\n    runs: [codex-mock/snapshots/runs/r1]\n' "$1" > spec/capabilities/x.yaml; }
statement "x works"; c base
BASE=$(git rev-parse HEAD)

# a recording of claude changes: only claude's pair is checked, and it is proven (codex's missing test is not this change's)
git checkout -q -b rec-claude "$BASE"
echo "version: 2" > claude-mock/snapshots/runs/r1/run.yaml; c "claude is recorded again"
expect_passed "$BASE" "a change that touches only claude"

# a recording of codex changes: codex's pair is checked, and no test proves it
git checkout -q -b rec-codex "$BASE"
echo "version: 2" > codex-mock/snapshots/runs/r1/run.yaml; c "codex is recorded again"
expect_refused "$BASE" "a change to codex's recording" "no test carries // sr:proves x/codex"

# the shared statement changes: every harness's pair is checked
git checkout -q -b statement "$BASE"
statement "x works, said another way"; c "the statement changes"
expect_refused "$BASE" "a change to the shared statement" "no test carries // sr:proves x/codex"

# recovery: codex gets its test, and the same range from the same base passes
mkdir -p codex-mock/e2e; printf 'package e2e\n\n// sr:proves x/codex\nfunc TestX() {}\n' > codex-mock/e2e/x_test.go; c "codex proof"
expect_passed "$BASE" "codex is proven"
