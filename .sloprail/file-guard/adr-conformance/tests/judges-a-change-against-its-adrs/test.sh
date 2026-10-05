#!/usr/bin/env bash
set -euo pipefail

# adr-conformance judges the production code a change touches against the ADRs that link the rule. The judge is a
# mock (SR_CHECKS_JUDGE_MOCKS) that reads the rendered prompt's diffs: a diff adding a FORBIDDEN line breaks the
# one ADR's decision, any other passes. The CI path, no agent turn: `sr-checks run` judges committed ranges with
# the project's rules; only this rule's outcome is asserted, one scenario per commit (a verdict is cached by
# content, and a stored refusal is replayed):
#   a change that breaks the decision          -> refused, with the judge's reason naming the file
#   the same file fixed (recovery)             -> passed
#   a conforming change                        -> passed
#   a test file, or code outside internal/ and the mocks (the match's edge), however FORBIDDEN -> not judged, no event
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
RULE=adr-conformance
git init -q -b main .
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
# only this rule runs: the others are switched off for the case
{ echo 'disabled:'; echo '  - sloprail/gate/ci-verify-required'; echo '  - sloprail/file-guard/rule-tests-pass'
  for d in .sloprail/file-guard/*/ .sloprail/gate/*/; do n="$(basename "$(dirname "$d")")/$(basename "$d")"; [ "$n" = "file-guard/$RULE" ] || echo "  - $n"; done; } >.sloprail/config.yaml
c() { git add -A && git commit -q -m "$1"; }
mkdir -p adr/quiet internal
printf -- '---\nconcern: code stays quiet\nsloprails: [file-guard/adr-conformance]\n---\n## Concern\nc\n## Decision\n- code never says FORBIDDEN\n' >adr/quiet/ADR.md
printf 'package internal\n\nfunc a() {}\n' >internal/a.go
c base
BASE=$(git rev-parse HEAD)
export SR_CHECKS_JUDGE_MOCKS='{"file-guard/adr-conformance/adr-conforms":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'

run_rule() {
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_passed() {   # LABEL
  jq -es --arg r "$RULE" '[.[] | select(.kind=="FileGuardChecked" and .rule==$r)] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING
  jq -es --arg r "$RULE" --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .rule==$r and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not refuse with a reason saying '$2' (sr-checks exit $ran)" >&2; exit 1; }
}
expect_not_judged() {   # LABEL
  jq -es --arg r "$RULE" '[.[] | select(.kind=="FileGuardChecked" and .rule==$r)] | length == 0' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE was asked about a change outside its match" >&2; exit 1; }
}

# a change that breaks the ADR's decision: refused with the judge's reason, naming the file
git checkout -q -b breaks "$BASE"
printf 'package internal\n\n// FORBIDDEN\nfunc a() {}\n' >internal/a.go; c "a says FORBIDDEN"
run_rule
expect_refused "a change that breaks the decision" "quiet #1: internal/a.go breaks"
# recovery: the agent removes the line, and the same range passes
printf 'package internal\n\nfunc a() {}\nfunc b() {}\n' >internal/a.go; c "a is fixed"
run_rule
expect_passed "the fixed change"

# a conforming change to production code is judged and passed
git checkout -q -b conforms "$BASE"
printf 'package internal\n\nfunc b() {}\n' >internal/b.go; c "b, conforming"
run_rule
expect_passed "a conforming change"

# the match's edge: a test file and code outside internal/ and the mocks are not production code the ADRs govern
git checkout -q -b outside "$BASE"
printf 'package internal\n\n// FORBIDDEN\nfunc TestA() {}\n' >internal/a_test.go
mkdir -p tools && printf 'package tools\n\n// FORBIDDEN\nfunc t() {}\n' >tools/t.go; c "FORBIDDEN in a test and in tools/"
run_rule
expect_not_judged "a test file and code outside the match"
