#!/usr/bin/env bash
set -euo pipefail
# The cited page also describes DOCONLY, which the statement does not claim and no test drives: not demanded.
. "$SR_TEST_CASE_DIR/scope-lib.sh"
scope_case "c proves alpha" "// asserts alpha" '{"e":"alpha"}'
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length==1 and .[0].outcome=="passed"' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "a doc behaviour outside the statement was demanded" >&2; exit 1; }
