#!/usr/bin/env bash
# The transcript module's own search for transcript logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: the chain fields and hook record types only a session's chained record file writes. Comments, test support and repo tooling are not candidates.
set -uo pipefail
patterns='"parentUuid"|"logicalParentUuid"|"hook_success"|"hook_blocking_error"|"stop_hook_summary"'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**' ':!tools/**' ':!*/e2etest/**' | grep -vE '^[^:]+:[0-9]+:[[:space:]]*//'
rc=${PIPESTATUS[0]}
[ "$rc" -le 1 ]    # 1 = no match: fine
