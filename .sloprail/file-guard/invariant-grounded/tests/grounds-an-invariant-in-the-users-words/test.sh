#!/usr/bin/env bash
set -euo pipefail

# invariant-grounded: adding, rewording or removing a spec/invariants/<id>.yaml needs the user's words on the commit
# (Sloprail-Cites-User) and a judge checks the statement says what the words ask. The judge is a mock
# (SR_CHECKS_JUDGE_MOCKS): a statement that adds a number of seconds the cited words do not state fails. The CI path:
# `sr-checks run` judges committed ranges with the project's rules (only this rule runs); its outcome is asserted on
# the FileGuardChecked events, one scenario per commit (a verdict is cached by content):
#   a new invariant with no citation                     -> refused, "must cite the user's own words"
#   cited, and the statement is what the words ask       -> passed
#   cited, but the statement adds a number of seconds    -> refused with the judge's reason; fixed and cited again (recovery) -> passed
#   a removal with no citation                           -> refused; cited -> passed
#   a file outside the match (a non-kebab name)          -> not judged
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
RULE=invariant-grounded
git init -q -b main .
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
# only this rule runs: the others are switched off for the case
{ echo 'disabled:'; echo '  - sloprail/gate/ci-verify-required'; echo '  - sloprail/file-guard/rule-tests-pass'
  for d in .sloprail/file-guard/*/ .sloprail/gate/*/; do n="$(basename "$(dirname "$d")")/$(basename "$d")"; [ "$n" = "file-guard/$RULE" ] || echo "  - $n"; done; } >.sloprail/config.yaml
c() { git add -A && git commit -q -m "$1"; }
WORDS="please add the x invariant, nothing more"
cited() { git add -A && git commit -q -m "$1" --trailer "Sloprail-Cites-User: $WORDS"; }
export SR_CHECKS_JUDGE_MOCKS='{"file-guard/invariant-grounded/statement-from-words":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'
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
  jq -es --arg r "$RULE" '[.[] | select(.kind=="FileGuardChecked" and .rule==$r)] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING...
  local label="$1" s; shift
  for s in "$@"; do
    jq -es --arg r "$RULE" --arg s "$s" 'any(.[]; .kind=="FileGuardChecked" and .rule==$r and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
      { jq -c . "$SR_EVENTS_FILE" >&2; echo "$label: $RULE did not refuse with a reason saying '$s' (sr-checks exit $ran)" >&2; exit 1; }
  done
}
expect_not_judged() {   # LABEL
  jq -es --arg r "$RULE" '[.[] | select(.kind=="FileGuardChecked" and .rule==$r)] | length == 0' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE was asked about a change outside its match" >&2; exit 1; }
}

mkdir -p spec/invariants
printf 'statement: it holds\n' >spec/invariants/base.yaml
echo base >README
c base
BASE=$(git rev-parse HEAD)

# a new invariant with no citation: refused for the missing words, however faithful
git checkout -q -b nowords "$BASE"
printf 'statement: x holds\n' >spec/invariants/x.yaml; c "x, no words from the user"
run_rule
expect_refused "a new invariant with no citation" "spec/invariants/x.yaml must cite the user's own words in the commit that last changed it"

# cited, and the statement is what the words ask: judged and passed
git checkout -q -b faithful "$BASE"
printf 'statement: x holds\n' >spec/invariants/x.yaml; cited "x, as asked"
run_rule
expect_passed "a cited, faithful statement"

# cited, but the statement adds a number the words do not state: refused with the judge's reason
git checkout -q -b unfaithful "$BASE"
printf 'statement: x holds within 5 seconds\n' >spec/invariants/x.yaml; cited "x, with a limit nobody asked for"
run_rule
expect_refused "a statement beyond the words" "the statement of x adds a number of seconds the words quoted do not state"
# recovery: the statement is brought back to the words, cited again, and the same range passes
printf 'statement: x holds\n' >spec/invariants/x.yaml; cited "x, as asked"
run_rule
expect_passed "the statement brought back to the words"

# a removal needs the words too: refused without them, passed with them
git checkout -q -b removal "$BASE"
git rm -q spec/invariants/base.yaml; c "base is dropped, no words"
run_rule
expect_refused "a removal with no citation" "spec/invariants/base.yaml must cite the user's own words in the commit that last changed it"
git commit -q --amend --no-edit --trailer "Sloprail-Cites-User: $WORDS"
run_rule
expect_passed "a cited removal"

# the match's edge: a file name that is not kebab-case is no invariant file here (invariant-covered and shapes refuse it)
git checkout -q -b outside "$BASE"
printf 'statement: odd\n' >spec/invariants/Odd_Name.yaml; c "a badly named file"
run_rule
expect_not_judged "a file outside the match"
