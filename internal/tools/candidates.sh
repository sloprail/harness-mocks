#!/usr/bin/env bash
# The tools module's own search for tools logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: the tool names a tool executor switches on and the input fields only the file and shell tools read. Comments, test support and repo tooling are not candidates.
set -uo pipefail
patterns='case "(Bash|Read|Write|Edit|Glob)"|"old_string"|"noOutputExpected"'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**' ':!tools/**' ':!*/e2etest/**' | grep -vE '^[^:]+:[0-9]+:[[:space:]]*//'
rc=${PIPESTATUS[0]}
[ "$rc" -le 1 ]    # 1 = no match: fine
