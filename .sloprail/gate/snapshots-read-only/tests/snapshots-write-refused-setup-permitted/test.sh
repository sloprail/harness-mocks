#!/usr/bin/env bash
set -euo pipefail
# A Write to a recorded sample under <harness>-mock/snapshots/ is refused, naming capture.sh; what a person authors, a
# scenario's setup/ and the capture script, is written (the gate does not wake for them: no event, and the files hold
# what was written).
git init -q -b main .
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests
# the structure gate also refuses a write under snapshots/ (only what a person authors is allowed), and speaks first: it is
# left out of the sandbox so that the decision under test is this gate's own
rm -f .sloprail/file-guard/structure.yaml
printf 'disabled:\n  - sloprail/gate/ci-verify-required\n  - sloprail/file-guard/rule-tests-pass\n' >.sloprail/config.yaml
mkdir -p claude-mock/snapshots/runs/r/samples/20240101-000000 claude-mock/snapshots/runs/r/setup
printf '{"e":1}\n' >claude-mock/snapshots/runs/r/samples/20240101-000000/events.jsonl
printf 'say hi\n' >claude-mock/snapshots/runs/r/setup/prompt.txt
printf '#!/bin/sh\nexit 0\n' >claude-mock/snapshots/capture.sh
git add -A && git -c user.name=t -c user.email=t@t commit -q -m init
RESULT=$(sr-test agent "$SR_TEST_CASE_DIR/agent.sh" --prompt "write into the snapshots")
echo "$RESULT" | jq -e '[.events[] | select(.kind=="GateChecked" and .rule=="snapshots-read-only" and .outcome=="refused")] | length==1 and .[0].tool_use_id=="t1" and (.[0].reason | contains("is a snapshot of the real harness: it is captured, not written"))' >/dev/null ||
  { echo "$RESULT" | jq -c .events >&2; echo "the write to a recorded sample was not the one refusal, naming capture.sh" >&2; exit 1; }
[ "$(cat claude-mock/snapshots/runs/r/samples/20240101-000000/events.jsonl)" = '{"e":1}' ] || { echo "the refused write changed the sample" >&2; exit 1; }
[ "$(cat claude-mock/snapshots/runs/r/setup/prompt.txt)" = 'say hello' ] || { echo "the write to setup/ did not land" >&2; exit 1; }
[ "$(cat claude-mock/snapshots/capture.sh)" = '#!/bin/sh' ] || { echo "the write to capture.sh did not land" >&2; exit 1; }
