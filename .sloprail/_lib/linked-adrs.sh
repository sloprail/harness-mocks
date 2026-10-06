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
# a failed count is not "nothing to judge": it refuses
n="$(jq 'length' <<<"$ADRS")" || refuse_error "the ADRs that link this rule could not be counted, so nothing could be judged"
[ "$n" -gt 0 ] || { jq -n '{skip: true}'; exit 0; }
n="$(cs '.changeset.files | length')" || refuse_error "the changeset's files could not be counted, so nothing could be judged"
[ "$n" -gt 0 ] || { jq -n '{skip: true}'; exit 0; }

dir="$(mktemp -d "${TMPDIR:-/tmp}/sr-judge-diffs.XXXXXX")" || refuse_error "cannot make a directory for the judge's diffs"
list="$(cs_json '.changeset.files[]')" || refuse_error "the changeset's files could not be listed, so nothing could be judged"
files="[]"
while IFS= read -r f; do
  [ -n "$f" ] || continue
  path="$(jq -r '.path' <<<"$f")" || refuse_error "a changed file's path could not be read"
  st="$(jq -r '.status' <<<"$f")" || refuse_error "$path: its status could not be read"
  old="$(jq -r '.oldPath // ""' <<<"$f")" || refuse_error "$path: its old path could not be read"
  out="$dir/$(printf '%s' "$path" | tr '/' '_').diff"
  jq -r '.diff // ""' <<<"$f" >"$out" || refuse_error "$path: its diff could not be written for the judge"
  files="$(jq -c --arg p "$path" --arg s "$st" --arg o "$old" \
    --arg d "$out" --arg a "$(grep -c '^+[^+]' "$out")" --arg r "$(grep -c '^-[^-]' "$out")" \
    '. + [{path: $p, status: $s, oldPath: $o, diff: $d, added: ($a | tonumber), removed: ($r | tonumber)}]' <<<"$files")" ||
    refuse_error "$path: it could not be listed for the judge"
done <<<"$list"

# the files and ADRs go to jq through files, not argv (Linux caps one argument at 128 KB)
adrs="$(jq -c '[.[] | {id, path}]' <<<"$ADRS")" || refuse_error "the linked ADRs could not be listed for the judge"
base="$(cs '.changeset.base')" || refuse_error "the range's base could not be read"
head="$(cs '.changeset.head')" || refuse_error "the range's head could not be read"
jq -n -c --arg rule "$(rule_qname)" --slurpfile a <(printf '%s' "$adrs") --slurpfile f <(printf '%s' "$files") \
  --arg base "$base" --arg head "$head" \
  '{additionalContext: {rule: $rule, adrs: $a[0], files: $f[0], base: $base, head: $head}}' ||
  refuse_error "the judge's input could not be built"
