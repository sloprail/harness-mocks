#!/usr/bin/env bash
set -euo pipefail
# beta is shown by the recording and by no test, but the run c is on the replay exception list (notReplaying): it
# does not replay green, so it proves nothing, and the clause is refused.
. "$SR_TEST_SLOPRAIL_DIR/file-guard/capability-rigor/test-lib/scope-lib.sh"
scope_case "c proves alpha beta" "// asserts alpha" '{"e":"alpha beta"}' "the replay differs from the recording"
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length==1 and .[0].outcome=="refused" and (.[0].reason | contains("no test or replay proves the clause beta"))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "a clause shown only by a run that does not replay was permitted" >&2; exit 1; }

# the fix the refusal asks for: a test asserting beta (the exception-listed run stays unable to prove it); judged
# again, and passed
add_test "// asserts beta"
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length==1 and .[0].outcome=="passed"' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "a test for the clause did not satisfy the rule" >&2; exit 1; }
