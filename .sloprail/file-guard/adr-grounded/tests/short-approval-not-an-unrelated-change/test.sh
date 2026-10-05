#!/usr/bin/env bash
set -euo pipefail

# The same "lgtm" does not ground a change the assistant never proposed: it approved 'retry-on-timeout', and the commit
# changes 'cache-everything'. The judge reads the record, sees the approval covers something else, and the change is refused
# with its reasoning. The scripts are the sibling case's, copied (a case sees only its own folder); CHANGE picks the
# unrelated change. Recovery: the agent drops the unrelated commit and makes the change that was proposed, citing the
# same reply, and the rule then permits it.
# the rules read the specs with yq, which the case's PATH does not carry
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
git init -q .
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
# only adr-grounded runs: every other rule of the project is switched off
{ echo 'disabled:'; echo '  - sloprail/gate/ci-verify-required'; echo '  - sloprail/file-guard/rule-tests-pass'
  for r in .sloprail/file-guard/*/ .sloprail/gate/*/; do n=${r#.sloprail/}; n=${n%/}; [ "$n" = file-guard/adr-grounded ] || echo "  - $n"; done
} > .sloprail/config.yaml
mkdir -p adr/existing; printf "# Existing\n" > adr/existing/ADR.md
git add -A
git -c user.name=t -c user.email=t@t commit -q -m "what already stands"
export SR_CHECKS_JUDGE_MOCKS='{"file-guard/adr-grounded/words-ask-for-adr":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'
export CHANGE=unrelated
SID=short-approval-session
sr-test agent "$SR_TEST_CASE_DIR/propose.sh" --session "$SID" --prompt "work on the project" >/dev/null
RESULT=$(sr-test agent "$SR_TEST_CASE_DIR/agent.sh" --session "$SID" --prompt "lgtm")
fail() { echo "$1" >&2; echo "$RESULT" | jq -c '.events[]|select(.rule=="adr-grounded")|{kind,rule,outcome,on,tool_use_id,reason}' >&2; exit 1; }
SESSION=$(echo "$RESULT" | jq -er .session)
echo "$RESULT" | jq -e '[.events[]|select(.kind=="FileGuardChecked" and .rule=="adr-grounded")] | (.[0].outcome=="refused" and (.[0].reason|contains("did not include cache-everything")))' >/dev/null || fail "adr-grounded did not refuse the unrelated change with the judge reasoning"
echo "$RESULT" | jq -e '[.events[]|select(.kind=="FileGuardChecked" and .rule=="adr-grounded")] | map(.outcome)==["refused","passed"]' >/dev/null || fail "adr-grounded did not then permit the change that was proposed"
jq -rs '[.[]|select(.type=="user")|.message.content[]?|select(.type=="tool_result" and .tool_use_id=="c2")][0]|.is_error' "$SESSION" | grep -qv true || fail "c2: the proposed change was not permitted after the refusal"
