#!/usr/bin/env bash
# The procexec module's own search for procexec logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: the process-group handling only starting and stopping a child process needs: becoming a group leader and signalling a group. Comments, test support and repo tooling are not candidates.
set -uo pipefail
patterns='\bSetpgid\b|syscall\.Kill\(|\.SysProcAttr\b'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**' ':!tools/**' ':!*/e2etest/**' | grep -vE '^[^:]+:[0-9]+:[[:space:]]*//'
rc=${PIPESTATUS[0]}
[ "$rc" -le 1 ]    # 1 = no match: fine
