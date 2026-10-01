#!/usr/bin/env bash
# The session module's own search for session logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: where a session's file lives and the failure of a resume that names no session. Comments, test support and repo tooling are not candidates.
set -uo pipefail
patterns='\.jsonl"|"forkedFrom"|No conversation found'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**' ':!tools/**' ':!*/e2etest/**' | grep -vE '^[^:]+:[0-9]+:[[:space:]]*//'
rc=${PIPESTATUS[0]}
[ "$rc" -le 1 ]    # 1 = no match: fine
