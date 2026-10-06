#!/usr/bin/env bash
set -euo pipefail
# Only runs already on main are read-only: a run directory that is not on main (the branch's own recording)
# may be deleted by hand, while a sample of a run on main is still refused, naming capture.sh.
git init -q -b main .
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests
printf 'disabled:\n  - sloprail/gate/ci-verify-required\n  - sloprail/file-guard/rule-tests-pass\n' >.sloprail/config.yaml
mkdir -p claude-mock/snapshots/runs/r/samples/20240101-000000
printf '{"e":1}\n' >claude-mock/snapshots/runs/r/samples/20240101-000000/events.jsonl
printf '#!/bin/sh\nexit 0\n' >claude-mock/snapshots/capture.sh
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run r is on main"
# a run that only this branch has, committed but not on main
git checkout -q -b feature
mkdir -p claude-mock/snapshots/runs/new/samples/20240101-000000
printf '{"e":2}\n' >claude-mock/snapshots/runs/new/samples/20240101-000000/events.jsonl
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run new is the branch's own"
RESULT=$(sr-test agent "$SR_TEST_CASE_DIR/agent.sh" --prompt "drop two samples by hand")
# t1 (the run not on main) is not refused; t2 (the run on main) is, naming capture.sh
echo "$RESULT" | jq -e '[.events[] | select(.kind=="GateChecked" and .rule=="snapshots-read-only" and .outcome=="refused")] | length==1 and .[0].tool_use_id=="t2" and (.[0].reason | contains("capture.sh"))' >/dev/null ||
  { echo "$RESULT" | jq -c .events >&2; echo "the run on main was not the only refusal" >&2; exit 1; }
# and the gate decided t1 itself: a GateChecked of this rule that permitted it
echo "$RESULT" | jq -e '[.events[] | select(.kind=="GateChecked" and .rule=="snapshots-read-only" and .tool_use_id=="t1")] | length==1 and .[0].outcome=="permitted"' >/dev/null ||
  { echo "$RESULT" | jq -c .events >&2; echo "the gate did not permit the deletion of the run that is not on main" >&2; exit 1; }
SESSION=$(echo "$RESULT" | jq -er .session)
jq -es '[.[] | select(.type=="user") | .message.content[]? | select(.type=="tool_result" and .tool_use_id=="t1")] | length==1 and all(.[]; .is_error!=true)' "$SESSION" >/dev/null ||
  { echo "the deletion of the run that is not on main did not run cleanly" >&2; exit 1; }
