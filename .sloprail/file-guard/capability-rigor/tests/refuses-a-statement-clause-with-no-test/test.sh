#!/usr/bin/env bash
set -euo pipefail
# The statement claims alpha and beta; a test drives alpha only and the recording shows no beta: refused.
. "$SR_TEST_CASE_DIR/scope-lib.sh"
scope_case "c proves alpha beta" "// asserts alpha" '{"e":"alpha"}'
verdict refused "a statement clause nothing proves was permitted"
