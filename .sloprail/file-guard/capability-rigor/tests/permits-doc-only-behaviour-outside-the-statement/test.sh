#!/usr/bin/env bash
set -euo pipefail
# The cited page also describes DOCONLY, which the statement does not claim and no test drives: not demanded.
. "$SR_TEST_CASE_DIR/scope-lib.sh"
scope_case "c proves alpha" "// asserts alpha" '{"e":"alpha"}'
verdict passed "a doc behaviour outside the statement was demanded"
