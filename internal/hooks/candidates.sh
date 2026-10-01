#!/usr/bin/env bash
# The hooks module's own search for hooks logic, anywhere in the tree.
# Contract: run from the tree's root; print `path:line:snippet` per candidate
# (git grep -n format); exit non-zero only if the search itself failed.
#
# Candidates: interpreting a handler's result (its exit code, a decision to
# block), the JSON keys only a hooks module reads, and every exported func of the module's internal packages
# called from anywhere (derived from the code, so it stays in sync).
set -uo pipefail
patterns='\.ExitCode\(\)|[eE]xitCode == 2|"decision"|"hookSpecificOutput"|"additionalContext"|"permissionDecision"|"stop_hook_active"'
funcs="$(go list -f '{{$d := .Dir}}{{range .GoFiles}}{{$d}}/{{.}} {{end}}' ./internal/hooks/internal/... 2>/dev/null |
  xargs grep -hoE '^func [A-Z][A-Za-z0-9]*' 2>/dev/null | sed -E 's/^func //' | paste -sd'|' -)"
[ -z "$funcs" ] || patterns="$patterns|\\b($funcs)\\("
git grep -n -I -E "$patterns" -- '*.go' ':!proposals/**'
rc=$?
[ "$rc" -le 1 ]    # 1 = no match: fine
