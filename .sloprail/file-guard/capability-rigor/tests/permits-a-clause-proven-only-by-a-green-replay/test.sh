#!/usr/bin/env bash
set -euo pipefail
# beta has no sr:proves assertion, but the recording shows it and its replay is green: proven.
. "$SR_TEST_CASE_DIR/scope-lib.sh"
scope_case "c proves alpha beta" "// asserts alpha" '{"e":"alpha beta"}'
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length==1 and .[0].outcome=="passed"' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "a clause the replay proves was refused" >&2; exit 1; }
