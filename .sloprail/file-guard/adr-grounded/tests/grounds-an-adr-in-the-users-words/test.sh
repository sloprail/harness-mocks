#!/usr/bin/env bash
set -euo pipefail

# adr-grounded: a new ADR, a changed decision, a new exception, a new or dropped rule link or a deleted ADR needs the
# user's words on the commit (Sloprail-Cites-User) and a judge checks the words ask for that decision; shrinking an
# exception list needs none. The judge is a mock (SR_CHECKS_JUDGE_MOCKS): an ADR that decides "forever" without the
# words saying so decides more than they do and fails. The CI path: `sr-checks run` judges committed ranges with the
# project's rules (only this rule runs); its outcome is asserted on the FileGuardChecked events, one scenario per
# commit (a verdict is cached by content):
#   a new ADR with no citation                           -> refused, "must cite the user's own words"
#   cited, and the ADR decides what the words ask        -> passed
#   cited, but the ADR decides more ("forever")          -> refused with the judge's reason; fixed and cited (recovery) -> passed
#   an exception added, no citation                      -> refused; an exception removed, no citation -> passed (waived)
#   a path under adr/ outside an ADR folder             -> not judged
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
RULE=adr-grounded
git init -q -b main .
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
# only this rule runs: the others are switched off for the case
{ echo 'disabled:'; echo '  - sloprail/gate/ci-verify-required'; echo '  - sloprail/file-guard/rule-tests-pass'
  for d in .sloprail/file-guard/*/ .sloprail/gate/*/; do n="$(basename "$(dirname "$d")")/$(basename "$d")"; [ "$n" = "file-guard/$RULE" ] || echo "  - $n"; done; } >.sloprail/config.yaml
c() { git add -A && git commit -q -m "$1"; }
WORDS="please record that handlers return errors"
cited() { git add -A && git commit -q -m "$1" --trailer "Sloprail-Cites-User: $WORDS"; }
export SR_CHECKS_JUDGE_MOCKS='{"file-guard/adr-grounded/words-ask-for-adr":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'
# the user asks (one agent turn puts their prompt in the session record), so the trailer's quote resolves
SETUP=$(sr-test agent "$SR_TEST_CASE_DIR/agent.sh" --prompt "$WORDS")
export CLAUDE_CONFIG_DIR=$(echo "$SETUP" | jq -er .config_dir)
export CLAUDE_CODE_PLUGIN_CACHE_DIR=$(echo "$SETUP" | jq -er .plugin_cache)
export CLAUDE_CODE_SESSION_ID=$(basename "$(echo "$SETUP" | jq -er .session)" .jsonl)
run_rule() {
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_passed() {   # LABEL
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="adr-grounded")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING...
  local label="$1" s; shift
  for s in "$@"; do
    jq -es --arg s "$s" 'any(.[]; .kind=="FileGuardChecked" and .rule=="adr-grounded" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
      { jq -c . "$SR_EVENTS_FILE" >&2; echo "$label: $RULE did not refuse with a reason saying '$s' (sr-checks exit $ran)" >&2; exit 1; }
  done
}
expect_not_judged() {   # LABEL
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="adr-grounded")] | length == 0' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE was asked about a change outside its match" >&2; exit 1; }
}
# adr FOLDER BULLET [EXCEPTION...] — an ADR.md with one Decision bullet and the listed exceptions
adr() {
  local f="$1" b="$2" ex=""; shift 2
  [ $# -gt 0 ] && { ex=$'exceptions:\n'; for e in "$@"; do ex="$ex  - $e"$'\n'; done; }
  mkdir -p "adr/$f"
  printf -- '---\nconcern: c\nsloprails: [file-guard/adr-grounded]\n%s---\n## Concern\nc\n## Decision\n- %s\n' "$ex" "$b" >"adr/$f/ADR.md"
}

adr base "handlers return errors" internal/old.go internal/older.go
echo base >README
c base
BASE=$(git rev-parse HEAD)

# a new ADR with no citation: refused for the missing words, however faithful
git checkout -q -b nowords "$BASE"
adr errors "handlers return errors"; c "an ADR, no words from the user"
run_rule
expect_refused "a new ADR with no citation" "must cite the user's own words in the commit that last changed it"

# cited, and the ADR decides what the words ask: judged and passed
git checkout -q -b faithful "$BASE"
adr errors "handlers return errors"; cited "an ADR, as asked"
run_rule
expect_passed "a cited, faithful ADR"

# cited, but the ADR decides more than the words: refused with the judge's reason
git checkout -q -b unfaithful "$BASE"
adr errors "handlers return errors forever"; cited "an ADR that goes beyond the ask"
run_rule
expect_refused "an ADR beyond the words" "adr/errors decides more than the words quoted: it says forever"
# recovery: the decision is brought back to the words, cited again, and the same range passes
adr errors "handlers return errors"; cited "the ADR as asked"
run_rule
expect_passed "the decision brought back to the words"

# an exception added needs the words; one removed (a legacy site brought into line) needs none
git checkout -q -b grow "$BASE"
adr base "handlers return errors" internal/old.go internal/older.go internal/newer.go; c "a new exception, no words"
run_rule
expect_refused "an exception added without words" "adr/base/ADR.md must cite the user's own words in the commit that last changed it"
git checkout -q -b shrink "$BASE"
adr base "handlers return errors" internal/old.go; c "a legacy exception goes"
run_rule
expect_passed "an exception removed"

# the match's edge: a file under adr/ that is not inside an ADR folder is no ADR change
git checkout -q -b outside "$BASE"
printf 'notes\n' >adr/README.md; c "a note beside the ADRs"
run_rule
expect_not_judged "a file outside the match"
