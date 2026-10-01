#!/usr/bin/env bash
# The session module's own search for session logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: what only finding, resuming and forking a session touches: the session file's location and the resume and fork markers.
set -uo pipefail
patterns='\.jsonl"|"--resume"|"--fork-session"|"--session-id"|\bForkFrom\b|"forkedFrom"|"No conversation found'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**'
rc=$?
[ "$rc" -le 1 ]    # 1 = no match: fine
