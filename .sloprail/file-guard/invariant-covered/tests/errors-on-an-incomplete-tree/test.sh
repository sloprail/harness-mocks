#!/usr/bin/env bash
set -euo pipefail

# invariant-covered when its tooling cannot read the tree, as against a real finding. The CI path, no
# agent turn. A rule that cannot work out what to check refuses and says "error": true, so the verdict is
# never cached (the engine reports "could not be evaluated"); a genuine finding is a plain refusal, a verdict.
#   1. healthy tree: passed
#   2. spec/invariants gone from the tree (an incomplete tree): refused with an error (no verdict)
#   3. spec/invariants holds no *.yaml (a failed listing): refused with an error (no verdict)
#   4. a real finding (an invariant marker naming a spec file that is not there while others are): a plain
#      refusal: a verdict, not an error
# the rules read the specs with yq, which the case's PATH (jq, git, bash, the sloprail binaries) does not carry
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
git init -q .
c() { git add -A && git -c user.name=t -c user.email=t@t commit -q -m "$1"; }
last_event() { jq -cs '[.[] | select(.kind=="FileGuardChecked" and .on=="sr-checks run" and .rule=="invariant-covered")] | last' "$SR_EVENTS_FILE"; }
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

mkdir -p internal/core spec/invariants
printf 'package core\n\n// sr:invariant a-rule\nfunc A() {}\n' > internal/core/a.go
printf 'package core\n\n// sr:proves a-rule\nfunc TestA() {}\n' > internal/core/a_test.go
printf 'statement: a holds\n' > spec/invariants/a-rule.yaml
c base
BASE=$(git rev-parse HEAD)

git checkout -q -b healthy "$BASE"
echo "// more" >> internal/core/a.go; c "touch the code"
expect_passed "$BASE" "a healthy tree"

git checkout -q -b gone "$BASE"
git rm -q -r spec/invariants; c "spec/invariants missing from the tree"
expect_error "$BASE" "no spec dir" "spec/invariants is not in the committed tree"

git checkout -q -b empty "$BASE"
git rm -q spec/invariants/a-rule.yaml; mkdir -p spec/invariants; touch spec/invariants/.gitkeep; c "spec/invariants without specs"
expect_error "$BASE" "no spec file" "lists no *.yaml"

git checkout -q -b real "$BASE"
printf 'package core\n\n// sr:invariant b-rule\nfunc B() {}\n' > internal/core/b.go; c "marker names a missing spec"
expect_fail "$BASE" "a real finding" "sr:invariant 'b-rule' names no spec/invariants/b-rule.yaml"
