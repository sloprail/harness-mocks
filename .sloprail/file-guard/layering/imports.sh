#!/usr/bin/env bash
# adr/layering's check: go list over the committed tree, every package's
# imports (a package's tests' too) against the two rules in the ADR. Deterministic; the whole module, because a
# change to go.mod or a moved package can break a rule in files it never touched.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
root="$(tree)"
mod="$(cd "$root" && go list -m 2>/dev/null)" || refuse_error "go list -m failed in the committed tree, so imports cannot be checked"
out="$(cd "$root" && go list -f '{{.ImportPath}}{{range .Imports}} {{.}}{{end}}{{range .TestImports}} {{.}}{{end}}{{range .XTestImports}} {{.}}{{end}}' ./... 2>&1)" ||
  refuse_error "go list failed in the committed tree, so imports cannot be checked: $out"
problems="$(printf '%s\n' "$out" | awk -v mod="$mod/" '
  function area(p,  r) { if (index(p, mod) != 1) return ""; r = substr(p, length(mod) + 1)
                         if (r ~ /^internal(\/|$)/) return "core"
                         if (match(r, /^[a-z0-9]+-mock/)) return substr(r, 1, RLENGTH); return "" }
  { from = area($1); if (from == "") next
    for (i = 2; i <= NF; i++) { to = area($i); if (to == "" || to == from || to == "core") continue
      if (from == "core") print "- " $1 " (core) imports " $i " (a mock)"
      else print "- " $1 " (" from ") imports " $i " (another mock, " to ")" } }')"
[ -z "$problems" ] && exit 0
refuse "adr/layering (mocks depend on core, never on each other):
${problems}
Move what both sides need into the shared internal/."
