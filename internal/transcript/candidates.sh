#!/usr/bin/env bash
# The transcript module's own search for transcript logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: what only a session's chained record file touches: the chain fields every record carries and the record types written.
set -uo pipefail
patterns='"parentUuid"|"logicalParentUuid"|"promptId"|"hook_success"|"hook_blocking_error"|"hook_non_blocking_error"|"stop_hook_summary"|"hookEvent"'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**'
rc=$?
[ "$rc" -le 1 ]    # 1 = no match: fine
