#!/usr/bin/env bash
set -euo pipefail
# A refusal for beta; then a change that touches none of the capability's tests, runs or cell. The re-run raises
# nothing new: the same single finding (sloprail/harness-mocks#236). The mock judge is a fixed rule over the
# statement, so this proves the handoff (the unrelated file reaches the judge as no clause) and not how a model
# keeps its findings stable, which needs an sr-eval fixture.
. "$SR_TEST_SLOPRAIL_DIR/file-guard/capability-rigor/test-lib/scope-lib.sh"
scope_case "c proves alpha beta" "// asserts alpha" '{"e":"alpha"}'
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length==1 and .[0].outcome=="refused" and (.[0].reason | contains("no test or replay proves the clause beta"))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "the first run did not refuse for beta" >&2; exit 1; }
first="$(jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")][0].reason' "$SR_EVENTS_FILE")"

# an unrelated file changes
printf 'notes\n' >notes.txt
git add -A && git commit -q -m "an unrelated change"
: >"$SR_EVENTS_FILE"
status=0; sr-checks run --base "$BASE" --head HEAD >out 2>&1 || status=$?
jq -es --argjson first "$first" '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length==1 and .[0].outcome=="refused" and .[0].reason==$first' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "the re-run raised something other than the first finding" >&2; exit 1; }

# the one finding fixed (a test asserting beta); judged again, and passed
add_test "// asserts beta"
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor")] | length==1 and .[0].outcome=="passed"' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "a test for the clause did not satisfy the rule" >&2; exit 1; }
