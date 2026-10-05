#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# capability-covered's outcome is asserted. The sr:provides half is capability-reconciled's.
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

# one harness, claude; capability x implemented once in internal/
mkdir -p claude-mock/snapshots/runs/r1 claude-mock/internal internal/core spec/capabilities
echo "version: 1" > claude-mock/snapshots/runs/r1/run.yaml
printf 'package core\n\n// sr:capability x\nfunc X() {}\n' > internal/core/x.go
printf 'package internal\n\n// sr:provides x/claude\nfunc adapter() {}\n' > claude-mock/internal/x.go
supported() { printf 'statement: x works\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/r1]\n' > spec/capabilities/x.yaml; }
printf 'statement: x works\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/r1]\n    deviations:\n      - adr: dev-adr\n        kind: mock-not-modeled\n        statement: the mock leaves something out\n' > spec/capabilities/x.yaml
mkdir -p claude-mock/e2e adr/dev-adr; printf 'package e2e\n\n// sr:proves x/claude\nfunc TestX() {}\n' > claude-mock/e2e/x_test.go
printf -- '---\nconcern: c\n---\n## Decision\n- d\n' > adr/dev-adr/ADR.md; c base
BASE=$(git rev-parse HEAD)

# the ADR a deviation names is deleted: refused, naming it
git checkout -q -b deleted "$BASE"
git rm -q -r adr/dev-adr; c "dev-adr deleted"
expect_refused "$BASE" "a deviation citing a deleted ADR" "deviates citing adr/dev-adr, which does not exist"

# recovery: the deviation names an ADR that exists (a new one), and the same range passes
mkdir -p adr/other-adr; printf -- '---\nconcern: c\n---\n## Decision\n- d\n' > adr/other-adr/ADR.md
sed 's/dev-adr/other-adr/' spec/capabilities/x.yaml > spec/capabilities/x.new && mv spec/capabilities/x.new spec/capabilities/x.yaml; c "the deviation names other-adr"
expect_passed "$BASE" "the deviation names an ADR that exists"
