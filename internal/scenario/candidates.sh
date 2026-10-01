#!/usr/bin/env bash
# The scenario module's own search for scenario logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: the scenario protocol's environment variables, which only driving the scenario script reads or sets. Comments, test support and repo tooling are not candidates.
set -uo pipefail
patterns='A10N_MOCK_[A-Z_]+'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**' ':!tools/**' ':!*/e2etest/**' | grep -vE '^[^:]+:[0-9]+:[[:space:]]*//'
rc=${PIPESTATUS[0]}
[ "$rc" -le 1 ]    # 1 = no match: fine
