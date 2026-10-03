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

groups="[]"
out="$(mktemp -d "${TMPDIR:-/tmp}/sr-judge-module-leaks.XXXXXX")" || refuse "cannot make a directory for the judge's matches"
while IFS= read -r m; do
  [ -n "$m" ] || continue
  dir="$(jq -r '.dir' <<<"$m")"
  want_subject "$dir" || continue
  leak_left "$m" || continue
  [ "$(jq 'length' <<<"$LEFT")" -gt 0 ] || continue
  mfile="$out/$(printf '%s' "$dir" | tr '/' '_').matches"
  jq -r '.[] | "\(.path):\(.line):\(.text)"' <<<"$LEFT" >"$mfile"
  groups="$(jq -c --arg d "$dir" --argjson m "$m" --arg f "$mfile" --argjson n "$(jq 'length' <<<"$LEFT")" \
    '. + [{module: $d, concern: $m.concern, home: $m.home, api: $m.api, matches: $f, count: $n}]' <<<"$groups")"
done < <(jq -c '.[]' <<<"$MODULES")
[ "$(jq 'length' <<<"$groups")" -gt 0 ] || { jq -n '{skip: true}'; exit 0; }
jq -n -c --argjson g "$groups" '{additionalContext: {modules: $g}}'
