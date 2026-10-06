#!/usr/bin/env bash
set -euo pipefail
# beta has no sr:proves assertion, but the recording shows it and its replay is green: proven.
. "$SR_TEST_CASE_DIR/scope-lib.sh"
scope_case "c proves alpha beta" "// asserts alpha" '{"e":"alpha beta"}'
verdict passed "a clause the replay proves was refused"
