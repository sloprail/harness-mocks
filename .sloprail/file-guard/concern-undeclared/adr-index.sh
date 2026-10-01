#!/usr/bin/env bash
# prepare: the index of decided concerns (every ADR's and every module's one-line
# concern; a module's concern is decided too, in its module.yaml), about 50 tokens
# an ADR where the full texts would be several hundred each, and every changed
# file with its diff written to a file outside the project (under a temp dir) for
# the judge to read. Nothing that can grow is put in the prompt.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/modules.sh"
load_adrs; load_modules
[ "$(cs '.changeset.files | length')" -gt 0 ] || { jq -n '{skip: true}'; exit 0; }
dir="$(mktemp -d "${TMPDIR:-/tmp}/sr-judge-diffs.XXXXXX")" || refuse "cannot make a directory for the judge's diffs"
files="[]"
while IFS= read -r f; do
  path="$(jq -r '.path' <<<"$f")"
  out="$dir/$(printf '%s' "$path" | tr '/' '_').diff"
  jq -r '.diff // ""' <<<"$f" >"$out"
  files="$(jq -c --arg p "$path" --arg s "$(jq -r '.status' <<<"$f")" --arg d "$out" \
    '. + [{path: $p, status: $s, diff: $d}]' <<<"$files")"
done < <(cs_json '.changeset.files[]')
jq -n -c --argjson a "$ADRS" --argjson m "$MODULES" --argjson f "$files" \
  '{additionalContext: {adrs: ([$a[] | {id: ("adr/" + .id), concern: (.frontmatter.concern // "")}] + [$m[] | {id: ("module " + .dir), concern}]), files: $f}}'
