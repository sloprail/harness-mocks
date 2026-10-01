#!/usr/bin/env bash
# The compaction module's own search for compaction logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: the JSON keys and frames only a compaction reads or writes.
set -uo pipefail
patterns='"compact_boundary"|"isCompactSummary"|"compactMetadata"|"PreCompact"|"PostCompact"|"preservedSegment"'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**'
rc=$?
[ "$rc" -le 1 ]    # 1 = no match: fine
