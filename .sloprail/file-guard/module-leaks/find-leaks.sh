#!/usr/bin/env bash
# prepare: steps 1 and 2 of module-leaks (leaks-lib.sh), for the module this check's subject names
# (subjects.sh: one per module with candidates left; every module when the rule runs unsplit).
# Emits {"skip": true} when nothing is left to judge, else the leftover candidates grouped by
# module, with the module's own concern (module.yaml: one line). The leftover candidates are
# written, one path:line:snippet per line, to a file outside the project (under a temp dir) that
# the judge reads; nothing that can grow is put in the prompt.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/modules.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
. "${SR_GUARDRAIL_DIR:-.}/leaks-lib.sh"
load_modules; load_adrs; leak_setup

# a lookup that fails refuses (leaks-lib.sh): never "nothing left to judge", which would let every
# leak through; the groups (they grow with the modules) go to jq by file, never on its command line
mlist="$(jq -c '.[]' <<<"$MODULES")" || refuse_error "could not list the modules, so their leaks cannot be found"
groups="[]"
out="$(mktemp -d "${TMPDIR:-/tmp}/sr-judge-module-leaks.XXXXXX")" || refuse_error "cannot make a directory for the judge's matches"
while IFS= read -r m; do
  [ -n "$m" ] || continue
  dir="$(jq -r '.dir' <<<"$m")" || refuse_error "could not read a module's directory, so its leaks cannot be found"
  want_subject "$dir" || continue
  leak_left "$m" || continue
  nleft="$(jq 'length' <<<"$LEFT")" || refuse_error "could not count the candidates of $dir"
  [ "$nleft" -gt 0 ] || continue
  mfile="$out/$(printf '%s' "$dir" | tr '/' '_').matches"
  jq -r '.[] | "\(.path):\(.line):\(.text)"' <<<"$LEFT" >"$mfile" || refuse_error "could not write the candidates of $dir for the judge"
  groups="$(jq -c --arg d "$dir" --argjson m "$m" --arg f "$mfile" --argjson n "$nleft" \
    '. + [{module: $d, concern: $m.concern, home: $m.home, api: $m.api, matches: $f, count: $n}]' <<<"$groups")" ||
    refuse_error "could not record the candidates of $dir for the judge"
done <<<"$mlist"
ngroups="$(jq 'length' <<<"$groups")" || refuse_error "could not count the modules with candidates left"
[ "$ngroups" -gt 0 ] || { jq -n '{skip: true}'; exit 0; }
printf '%s' "$groups" >"$LEAK_WORK/groups"
jq -n -c --slurpfile g "$LEAK_WORK/groups" '{additionalContext: {modules: $g[0]}}' || refuse_error "could not build the judge's context"
