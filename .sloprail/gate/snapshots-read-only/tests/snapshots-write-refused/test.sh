#!/usr/bin/env bash
set -euo pipefail
# The project's own rules apply: its .sloprail is the one under test. Deleting a recorded sample by hand is
# refused, naming capture.sh (which changes what is recorded, and re-freezes the docs with it); the recovery
# the reason names (running capture.sh) and the boundary (a scenario's setup/, authored by hand) are not
# refused: the gate does not wake for them, no event, and the calls ran.
git init -q -b main .
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests
# the plugin's own Stop notices are incidental here
printf 'disabled:\n  - sloprail/gate/ci-verify-required\n  - sloprail/file-guard/rule-tests-pass\n' >.sloprail/config.yaml
mkdir -p claude-mock/snapshots/runs/r/samples/20240101-000000 claude-mock/snapshots/runs/r/setup
printf '{"e":1}\n' >claude-mock/snapshots/runs/r/samples/20240101-000000/events.jsonl
printf 'say hi\n' >claude-mock/snapshots/runs/r/setup/prompt.txt
printf '#!/bin/sh\nexit 0\n' >claude-mock/snapshots/capture.sh
git add -A && git -c user.name=t -c user.email=t@t commit -q -m init
RESULT=$(sr-test agent "$SR_TEST_CASE_DIR/agent.sh" --prompt "drop the sample by hand")
echo "$RESULT" | jq -e '[.events[] | select(.kind=="GateChecked" and .rule=="snapshots-read-only")] | length==1 and .[0].outcome=="refused" and .[0].tool_use_id=="t1" and (.[0].reason | contains("capture.sh"))' >/dev/null ||
  { echo "$RESULT" | jq -c .events >&2; exit 1; }
# no event of this rule for t2 (the recovery) or t3 (the boundary), and both calls ran without an error
# the gate refuses exactly one call, t1: for t2 and t3 it emits no refused GateChecked (the engine emits no event for a call a gate's match leaves alone)
echo "$RESULT" | jq -e '[.events[] | select(.rule=="snapshots-read-only" and .outcome=="refused")] | length==1 and .[0].tool_use_id=="t1"' >/dev/null
SESSION=$(echo "$RESULT" | jq -er .session)
jq -es '[.[] | select(.type=="user") | .message.content[]? | select(.type=="tool_result" and (.tool_use_id=="t2" or .tool_use_id=="t3"))] | length==2 and all(.[]; .is_error!=true)' "$SESSION" >/dev/null ||
  { echo "the recovery or the setup/ deletion did not run cleanly" >&2; exit 1; }
