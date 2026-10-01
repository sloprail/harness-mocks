#!/usr/bin/env bash
# The subagents module's own search for subagents logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: what only a nested agent run touches: its id and type, its sidechain transcript, its worktree and its dispatch fields.
set -uo pipefail
patterns='"agent_id"|"agentId"|"agent_type"|"agent_transcript_path"|"subagents"|"isSidechain"|"isolation"|"subagent_type"'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**'
rc=$?
[ "$rc" -le 1 ]    # 1 = no match: fine
