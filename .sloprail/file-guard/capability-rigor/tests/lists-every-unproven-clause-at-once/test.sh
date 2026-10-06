#!/usr/bin/env bash
set -euo pipefail
# The statement claims alpha, beta, gamma and delta and a test drives alpha only: the one refusal names every
# unproven clause (beta, gamma, delta), not just the first (sloprail/harness-mocks#236).
. "$SR_TEST_SLOPRAIL_DIR/file-guard/capability-rigor/test-lib/scope-lib.sh"
scope_case "c proves alpha beta gamma delta" "// asserts alpha" '{"e":"alpha"}'
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length==1 and .[0].outcome=="refused" and (.[0].reason | contains("the clause beta") and contains("the clause gamma") and contains("the clause delta"))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "the refusal did not name every unproven clause at once" >&2; exit 1; }

# all fixed in one pass: a test asserting the three clauses; judged again, and passed
add_test "// asserts beta gamma delta"
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length==1 and .[0].outcome=="passed"' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "one pass over the named clauses did not satisfy the rule" >&2; exit 1; }
