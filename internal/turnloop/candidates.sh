#!/usr/bin/env bash
# The turnloop module's own search for turnloop logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: what only the turn touches: the end-of-turn hook payload fields, the re-prompt on a block and its cap.
set -uo pipefail
patterns='"stop_hook_active"|STOP_HOOK_BLOCK_CAP|"prevent_continuation"|"preventedContinuation"|"stop_reason"|maxIdenticalTurns|"num_turns"'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**'
rc=$?
[ "$rc" -le 1 ]    # 1 = no match: fine
