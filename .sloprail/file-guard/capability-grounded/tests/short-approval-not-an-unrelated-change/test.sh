#!/usr/bin/env bash
set -euo pipefail

# The same "lgtm" does not ground a change the assistant never proposed: it approved 'tool-retry', and the commit
# changes 'cache-hit'. The judge reads the record, sees the approval covers something else, and the change is refused
# with its reasoning. The scripts are the sibling case's, copied (a case sees only its own folder); CHANGE picks the
# unrelated change.
# the rules read the specs with yq, which the case's PATH does not carry
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
git init -q .
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
# only capability-grounded runs: every other rule of the project is switched off
{ echo 'disabled:'; echo '  - sloprail/gate/ci-verify-required'; echo '  - sloprail/file-guard/rule-tests-pass'
  for r in .sloprail/file-guard/*/ .sloprail/gate/*/; do n=${r#.sloprail/}; n=${n%/}; [ "$n" = file-guard/capability-grounded ] || echo "  - $n"; done
} > .sloprail/config.yaml
mkdir -p spec/capabilities; for c in tool-retry cache-hit; do printf "statement: a thing works\nproviders:\n  claude: pending\n" > spec/capabilities/$c.yaml; done
git add -A
git -c user.name=t -c user.email=t@t commit -q -m "what already stands"
export SR_CHECKS_JUDGE_MOCKS='{"file-guard/capability-grounded/docs-support-statement":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'
export CHANGE=unrelated
SID=short-approval-session
sr-test agent "$SR_TEST_CASE_DIR/propose.sh" --session "$SID" --prompt "work on the project" >/dev/null
RESULT=$(sr-test agent "$SR_TEST_CASE_DIR/agent.sh" --session "$SID" --prompt "lgtm")
fail() { echo "$1" >&2; echo "$RESULT" | jq -c '.events[]|select(.rule=="capability-grounded")|{kind,rule,outcome,on,tool_use_id,reason}' >&2; exit 1; }
SESSION=$(echo "$RESULT" | jq -er .session)
echo "$RESULT" | jq -e '[.events[]|select(.kind=="FileGuardChecked" and .rule=="capability-grounded")] | any(.[]; .outcome=="refused" and (.reason|contains("did not include cache-hit")))' >/dev/null || fail "capability-grounded did not refuse the unrelated change with the judge reasoning"
