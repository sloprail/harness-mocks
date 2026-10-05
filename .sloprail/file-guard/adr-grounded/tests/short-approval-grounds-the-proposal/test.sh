#!/usr/bin/env bash
set -euo pipefail

# A short approval is read against what it answered. The assistant proposed one change, a few messages before, the user
# said "lgtm", and the commit cites "lgtm" (the only authority; the assistant message is context the judge reads
# from the source path:line it is handed): the change that was proposed is grounded and permitted.
# The judge is a mock that reads the record from that path:line, so this proves the handoff and the verdict path;
# how the real model reads the rubric is for sr-eval.
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

SID=short-approval-session
sr-test agent "$SR_TEST_CASE_DIR/propose.sh" --session "$SID" --prompt "work on the project" >/dev/null
RESULT=$(sr-test agent "$SR_TEST_CASE_DIR/agent.sh" --session "$SID" --prompt "lgtm")
fail() { echo "$1" >&2; echo "$RESULT" | jq -c '.events[]|select(.rule=="adr-grounded")|{kind,rule,outcome,on,tool_use_id,reason}' >&2; exit 1; }
SESSION=$(echo "$RESULT" | jq -er .session)
jq -rs '[.[]|select(.type=="user")|.message.content[]?|select(.type=="tool_result" and .tool_use_id=="c1")][0]|.is_error==false' "$SESSION" | grep -qx true || fail "c1: the approved change was not permitted"
echo "$RESULT" | jq -e '[.events[]|select(.kind=="FileGuardChecked" and .rule=="adr-grounded")] | length>0 and all(.[]; .outcome=="passed")' >/dev/null || fail "adr-grounded did not pass the approved change"
