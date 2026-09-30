#!/usr/bin/env bash
# SAMPLE. Prints one extended regex per line: code outside this module's home
# matching one is doing hooks work (a leak, unless it only uses the API).
# Run from the repository root of the tree being judged.
#
# Derived from the module's own code where possible, so it stays in sync:
# every exported func of its internal packages.
go list -f '{{$d := .Dir}}{{range .GoFiles}}{{$d}}/{{.}} {{end}}' ./core/hooks/internal/... 2>/dev/null |
  xargs grep -hoE '^func [A-Z][A-Za-z0-9]*' 2>/dev/null | sed -E 's/^func (.*)$/\\b\1\\(/' | sort -u
# The JSON a hook handler prints: only the module interprets it.
echo '"additionalContext"'
echo '"permissionDecision"'
echo '"stop_hook_active"'
# Reading a handler's result directly.
echo '\.ExitCode\(\)'
