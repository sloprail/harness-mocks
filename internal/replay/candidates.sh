#!/usr/bin/env bash
# The replay module's own search for replay logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: reads of a recording's own output files (stream.jsonl,
# payloads.jsonl), which only comparing a replay with its recording needs.
# Comments, other tests and repo tooling are not candidates.
set -uo pipefail
patterns='(stream|payloads)\.jsonl'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**' ':!tools/**' ':!*/e2etest/**' ':!*_test.go' | grep -vE '^[^:]+:[0-9]+:[[:space:]]*//'
rc=${PIPESTATUS[0]}
[ "$rc" -le 1 ]    # 1 = no match: fine
