#!/usr/bin/env bash
set -euo pipefail
# The statement claims alpha and beta; a test drives alpha only and the recording shows no beta: refused.
. "$SR_TEST_CASE_DIR/scope-lib.sh"
scope_case "c proves alpha beta" "// asserts alpha" '{"e":"alpha"}'
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length==1 and .[0].outcome=="refused" and (.[0].reason | contains("no test or replay proves the clause beta"))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "a statement clause nothing proves was permitted" >&2; exit 1; }
