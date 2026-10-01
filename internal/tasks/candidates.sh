#!/usr/bin/env bash
# The tasks module's own search for tasks logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: the frames and fields only background-task tracking handles.
set -uo pipefail
patterns='"task_started"|"task_notification"|"task_updated"|"run_in_background"|"is_backgrounded"|"background_tasks"|<task-notification>'
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**'
rc=$?
[ "$rc" -le 1 ]    # 1 = no match: fine
