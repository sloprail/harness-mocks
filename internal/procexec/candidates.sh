#!/usr/bin/env bash
# The procexec module's own search for procexec logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: what only starting and stopping a child process touches: building a command, its process group, signalling a group, and the environment it is handed.
set -uo pipefail
patterns='exec\.Command(Context)?\(|\bSetpgid\b|syscall\.Kill\(|\.SysProcAttr\b|\bcmd\.Env\b|os\.Environ\(\)'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**'
rc=$?
[ "$rc" -le 1 ]    # 1 = no match: fine
