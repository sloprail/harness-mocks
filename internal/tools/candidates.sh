#!/usr/bin/env bash
# The tools module's own search for tools logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: what only executing a built-in tool touches: the tool names and the input fields the file and shell tools read.
set -uo pipefail
patterns='case "(Bash|Read|Write|Edit|Glob)"|"file_path"|"old_string"|"new_string"|"run_in_background"|"noOutputExpected"'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**'
rc=$?
[ "$rc" -le 1 ]    # 1 = no match: fine
