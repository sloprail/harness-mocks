#!/usr/bin/env bash
# prepare for a rule's ADR judge: names (by path, never inlined) every ADR that
# links this rule (many-to-many) and every changed file, with its diff written
# to a file outside the project for the judge to read. Nothing that can grow is
# put in the prompt: the judge reads what it needs. A rule no ADR links has
# nothing to conform to: that is adr-linked's finding, and the judge is
# skipped rather than run against nothing.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
load_adrs "$(rule_qname)"
[ "$(jq 'length' <<<"$ADRS")" -gt 0 ] || { jq -n '{skip: true}'; exit 0; }
[ "$(cs '.changeset.files | length')" -gt 0 ] || { jq -n '{skip: true}'; exit 0; }

dir="$(mktemp -d "${TMPDIR:-/tmp}/sr-judge-diffs.XXXXXX")" || refuse "cannot make a directory for the judge's diffs"
files="[]"
while IFS= read -r f; do
  path="$(jq -r '.path' <<<"$f")"
  out="$dir/$(printf '%s' "$path" | tr '/' '_').diff"
  jq -r '.diff // ""' <<<"$f" >"$out"
  files="$(jq -c --arg p "$path" --arg s "$(jq -r '.status' <<<"$f")" --arg o "$(jq -r '.oldPath // ""' <<<"$f")" \
    --arg d "$out" --arg a "$(grep -c '^+[^+]' "$out")" --arg r "$(grep -c '^-[^-]' "$out")" \
    '. + [{path: $p, status: $s, oldPath: $o, diff: $d, added: ($a | tonumber), removed: ($r | tonumber)}]' <<<"$files")"
done < <(cs_json '.changeset.files[]')

jq -n -c --arg rule "$(rule_qname)" --argjson a "$(jq -c '[.[] | {id, path}]' <<<"$ADRS")" --argjson f "$files" \
  --arg base "$(cs '.changeset.base')" --arg head "$(cs '.changeset.head')" \
  '{additionalContext: {rule: $rule, adrs: $a, files: $f, base: $base, head: $head}}'
