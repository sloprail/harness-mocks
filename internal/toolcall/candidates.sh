#!/usr/bin/env bash
# The toolcall module's own search for toolcall logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: the fields only a tool call's lifecycle carries: the tool-use id, the structured result and the empty-result placeholder. Comments, test support and repo tooling are not candidates.
set -uo pipefail
patterns='"tool_use_id"|"toolUseResult"|completed with no output'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**' ':!tools/**' ':!*/e2etest/**' | grep -vE '^[^:]+:[0-9]+:[[:space:]]*//'
rc=${PIPESTATUS[0]}
[ "$rc" -le 1 ]    # 1 = no match: fine
