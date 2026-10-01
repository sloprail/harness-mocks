#!/usr/bin/env bash
# The toolcall module's own search for toolcall logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: what only a tool call's lifecycle touches: the tool_use and tool_result blocks, their ids and the empty-result placeholder.
set -uo pipefail
patterns='"tool_use_id"|"tool_use"|"tool_result"|"toolUseResult"|completed with no output|"is_error"'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**'
rc=$?
[ "$rc" -le 1 ]    # 1 = no match: fine
