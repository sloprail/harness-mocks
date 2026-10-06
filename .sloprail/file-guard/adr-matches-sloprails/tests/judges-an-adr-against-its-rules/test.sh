#!/usr/bin/env bash
set -euo pipefail

# adr-matches-sloprails: an ADR and the sloprails it links say the same thing; re-judged when the ADR changes, or when
# any rule it links changes, so neither drifts alone. The judge is a mock (SR_CHECKS_JUDGE_MOCKS): an ADR whose
# Decision says FORBIDDEN is enforced only if a file of a linked rule says FORBIDDEN too. The CI path, no agent turn:
# `sr-checks run` judges committed ranges with the project's rules (only this rule runs); its outcome is asserted on
# the FileGuardChecked events, one scenario per commit (a verdict is cached by content):
#   a rule dropping what the ADR decides            -> refused with the judge's reason; the rule says it again (recovery) -> passed
#   a new ADR whose decision no linked rule checks   -> refused; one a linked rule does check -> passed
#   a change to a rule no ADR links                  -> no ADR to judge, nothing refused
#   a change outside adr/ and .sloprail/             -> not judged
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
RULE=adr-matches-sloprails
git init -q -b main .
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
# only this rule runs: the others are switched off for the case
{ echo 'disabled:'; echo '  - sloprail/gate/ci-verify-required'; echo '  - sloprail/file-guard/rule-tests-pass'
  for d in .sloprail/file-guard/*/ .sloprail/gate/*/; do n="$(basename "$(dirname "$d")")/$(basename "$d")"; [ "$n" = "file-guard/$RULE" ] || echo "  - $n"; done; } >.sloprail/config.yaml
c() { git add -A && git commit -q -m "$1"; }
export SR_CHECKS_JUDGE_MOCKS='{"file-guard/adr-matches-sloprails/rules-enforce-decision":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'
run_rule() {
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_passed() {   # LABEL
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="adr-matches-sloprails")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING...
  local label="$1" s; shift
  for s in "$@"; do
    jq -es --arg s "$s" 'any(.[]; .kind=="FileGuardChecked" and .rule=="adr-matches-sloprails" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
      { jq -c . "$SR_EVENTS_FILE" >&2; echo "$label: $RULE did not refuse with a reason saying '$s' (sr-checks exit $ran)" >&2; exit 1; }
  done
}
expect_not_refused() {   # LABEL
  jq -es 'all(.[] | select(.kind=="FileGuardChecked" and .rule=="adr-matches-sloprails"); .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE refused" >&2; exit 1; }
}
expect_not_judged() {   # LABEL
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="adr-matches-sloprails")] | length == 0' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE was asked about a change outside its match" >&2; exit 1; }
}
# adr FOLDER LINK BULLET — an ADR.md linking the rule LINK with one Decision bullet
adr() {
  mkdir -p "adr/$1"
  printf -- '---\nconcern: c\nsloprails: [%s]\n---\n## Concern\nc\n## Decision\n- %s\n' "$2" "$3" >"adr/$1/ADR.md"
}
SIZE=.sloprail/file-guard/file-size/file-guard.yaml

# the base: the ADR "strict" decides FORBIDDEN and its linked rule (file-size) says so
printf '\n# refuses FORBIDDEN\n' >>"$SIZE"
adr strict file-guard/file-size "code never says FORBIDDEN"
echo base >README
c base
BASE=$(git rev-parse HEAD)

# a linked rule drops what the ADR decides: the ADR is re-judged, refused with the judge's reason
git checkout -q -b drift "$BASE"
sed -i '' '/FORBIDDEN/d' "$SIZE"; c "the rule forgets"
run_rule
expect_refused "a rule that dropped the decision" "a Decision no linked rule checks: adr/strict says FORBIDDEN and no linked rule does"
# recovery: the rule says it again, and the same range passes
printf '\n# FORBIDDEN is refused\n' >>"$SIZE"; c "the rule says it again"
run_rule
expect_passed "the rule enforces the decision again"

# a new ADR whose decision no linked rule checks: refused; one a linked rule does check: passed
git checkout -q -b unchecked "$BASE"
adr wish file-guard/shapes "code never says FORBIDDEN"; c "an ADR nothing enforces"
run_rule
expect_refused "an ADR no linked rule enforces" "adr/wish says FORBIDDEN and no linked rule does"
git checkout -q -b checked "$BASE"
adr second file-guard/file-size "code never says FORBIDDEN"; c "a second ADR the rule enforces"
run_rule
expect_passed "an ADR a linked rule enforces"

# the boundary: a change to a rule no ADR links has no ADR to judge, and refuses nothing
git checkout -q -b unlinked "$BASE"
printf '\n# a note\n' >>.sloprail/file-guard/layering/file-guard.yaml; c "an unlinked rule changes"
run_rule
expect_not_refused "a change to a rule no ADR links"
# the match's edge: a change outside adr/ and .sloprail/ is not judged
git checkout -q -b outside "$BASE"
echo more >>README; c "the readme changes"
run_rule
expect_not_judged "a change outside the match"
