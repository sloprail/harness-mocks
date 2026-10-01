#!/usr/bin/env bash
# The turnloop module's own search for turnloop logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: the end-of-turn payload fields, the re-prompt marker and the block cap only the turn handles. Comments, test support and repo tooling are not candidates.
set -uo pipefail
patterns='"stop_hook_active"|STOP_HOOK_BLOCK_CAP|"prevent_continuation"|"preventedContinuation"|maxIdenticalTurns'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**' ':!tools/**' ':!*/e2etest/**' | grep -vE '^[^:]+:[0-9]+:[[:space:]]*//'
rc=${PIPESTATUS[0]}
[ "$rc" -le 1 ]    # 1 = no match: fine
