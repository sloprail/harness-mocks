#!/usr/bin/env bash
# The subagents module's own search for subagents logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: the fields only a nested agent run carries: its id, its sidechain flag and its dispatch type. Comments, test support and repo tooling are not candidates.
set -uo pipefail
patterns='"agent_id"|"agentId"|"agent_transcript_path"|"isSidechain"|"subagent_type"'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**' ':!tools/**' ':!*/e2etest/**' | grep -vE '^[^:]+:[0-9]+:[[:space:]]*//'
rc=${PIPESTATUS[0]}
[ "$rc" -le 1 ]    # 1 = no match: fine
