#!/usr/bin/env bash
set -euo pipefail

# capability-covered when its tooling cannot read the tree, as against a real finding. The CI path, no
# agent turn. A rule that cannot work out what to check refuses and says "error": true, so the verdict is
# never cached (the engine reports "could not be evaluated"); a genuine finding is a plain refusal, a verdict.
#   1. healthy tree: passed
#   2. spec/capabilities gone from the tree (an incomplete tree): refused with an error (no verdict)
#   3. spec/capabilities holds no *.yaml (a failed listing): refused with an error (no verdict)
#   4. a real finding (a cell with no proving test): a plain refusal: a verdict, not an error
# the rules read the specs with yq, which the case's PATH (jq, git, bash, the sloprail binaries) does not carry
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
git init -q .
c() { git add -A && git -c user.name=t -c user.email=t@t commit -q -m "$1"; }
last_event() { jq -cs '[.[] | select(.kind=="FileGuardChecked" and .on=="sr-checks run" and .rule=="capability-covered")] | last' "$SR_EVENTS_FILE"; }
judge() {   # BASE: run the range
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$1" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_passed() {   # BASE LABEL
  judge "$1"
  last_event | jq -e '.outcome=="passed"' >/dev/null || { last_event >&2; echo "$2: did not pass" >&2; exit 1; }
}
expect_error() {   # BASE LABEL SUBSTRING
  judge "$1"
  last_event | jq -e --arg s "$3" '.outcome=="refused" and (.reason|contains($s))' >/dev/null || { last_event >&2; echo "$2: not refused saying '$3'" >&2; exit 1; }
  # the engine words a check's "error": true as "could not be evaluated": no verdict, so nothing is cached
  last_event | jq -e '.reason | contains("could not be evaluated")' >/dev/null || { last_event >&2; echo "$2: not reported as an error (no verdict)" >&2; exit 1; }
}
expect_fail() {   # BASE LABEL SUBSTRING
  judge "$1"
  last_event | jq -e --arg s "$3" '.outcome=="refused" and (.reason|contains($s))' >/dev/null || { last_event >&2; echo "$2: not refused saying '$3'" >&2; exit 1; }
  last_event | jq -e '.reason | contains("could not be evaluated") | not' >/dev/null || { last_event >&2; echo "$2: a real finding was reported as an error" >&2; exit 1; }
}

mkdir -p claude-mock/snapshots/runs/r1 claude-mock/internal claude-mock/e2e internal/core spec/capabilities
echo "version: 1" > claude-mock/snapshots/runs/r1/run.yaml
printf 'package core\n\n// sr:capability x\nfunc X() {}\n' > internal/core/x.go
printf 'package internal\n\n// sr:provides x/claude\nfunc adapter() {}\n' > claude-mock/internal/x.go
printf 'package e2e\n\n// sr:proves x/claude\nfunc TestX() {}\n' > claude-mock/e2e/x_test.go
printf 'statement: x works\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/r1]\n' > spec/capabilities/x.yaml
c base
BASE=$(git rev-parse HEAD)

git checkout -q -b healthy "$BASE"
echo "// more" >> internal/core/x.go; c "touch the code"
expect_passed "$BASE" "a healthy tree"

git checkout -q -b gone "$BASE"
git rm -q -r spec/capabilities; c "spec/capabilities missing from the tree"
expect_error "$BASE" "no spec dir" "spec/capabilities is not in the committed tree"

git checkout -q -b empty "$BASE"
git rm -q spec/capabilities/x.yaml; mkdir -p spec/capabilities; touch spec/capabilities/.gitkeep; c "spec/capabilities without specs"
expect_error "$BASE" "no spec file" "lists no *.yaml"

git checkout -q -b real "$BASE"
git rm -q claude-mock/e2e/x_test.go; c "the proof is gone"
expect_fail "$BASE" "a real finding" "no test carries // sr:proves x/claude"
