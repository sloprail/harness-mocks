#!/usr/bin/env bash
# The scenario module's own search for scenario logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: what only driving the scenario script and routing its records touches: the script's environment variables, its path and the record kinds it may emit.
set -uo pipefail
patterns='A10N_MOCK_[A-Z_]+|\bScriptPath\b|"--script"'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**'
rc=$?
[ "$rc" -le 1 ]    # 1 = no match: fine
