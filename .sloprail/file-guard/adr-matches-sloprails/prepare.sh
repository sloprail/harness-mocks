#!/usr/bin/env bash
# prepare: one subject per ADR that changed, or that links a rule whose folder
# changed, as additionalContext.subjects (the engine does not split subjects
# yet; one judge call reviews them all).
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
load_adrs
changed="$(cs '.changeset.files[].path')"
subjects="[]"
while IFS= read -r a; do
  [ -n "$a" ] || continue
  id="$(jq -r '.id' <<<"$a")"
  links="$(jq -r '.frontmatter.sloprails // [] | .[]' <<<"$a")"
  hit=0
  printf '%s\n' "$changed" | grep -q "^adr/$id/" && hit=1
  while IFS= read -r l; do
    [ -n "$l" ] && printf '%s\n' "$changed" | grep -Fq ".sloprail/$l/" && hit=1
  done <<<"$links"
  [ "$hit" = 1 ] || continue
  rules="[]"
  while IFS= read -r l; do
    [ -n "$l" ] || continue
    dir="$SR_TREE/.sloprail/$l"
    [ -d "$dir" ] || continue                        # a dangling link is adr-linked's finding
    for f in "$dir"/*; do
      [ -f "$f" ] || continue
      rules="$(jq -c --arg r "$l" --arg p ".sloprail/$l/$(basename "$f")" --rawfile t "$f" '. + [{rule: $r, path: $p, text: $t}]' <<<"$rules")"
    done
  done <<<"$links"
  subjects="$(jq -c --arg id "$id" --argjson a "$a" --argjson r "$rules" \
    '. + [{id: $id, files: ([$a.path] + [$r[].path]), context: {adr: $a.text, rules: $r}}]' <<<"$subjects")"
done < <(jq -c '.[]' <<<"$ADRS")
if [ "$(jq 'length' <<<"$subjects")" -eq 0 ]; then echo '{"skip": true}'; exit 0; fi
jq -n -c --argjson s "$subjects" '{additionalContext: {subjects: $s}}'
