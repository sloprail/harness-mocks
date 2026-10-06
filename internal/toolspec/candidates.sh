#!/usr/bin/env bash
# The toolspec module's own search for tool-call checking logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: the words a mock refuses a script's tool call in. Comments, tests and repo tooling are not candidates.
set -uo pipefail
patterns='unknown tool|unknown parameter|missing required parameter'
git grep -n -I -i -E "$patterns" -- '*.go' ':!proposals/**' ':!tools/**' ':!*/e2etest/**' ':!*_test.go' | grep -vE '^[^:]+:[0-9]+:[[:space:]]*//'
rc=${PIPESTATUS[0]}
[ "$rc" -le 1 ]    # 1 = no match: fine
