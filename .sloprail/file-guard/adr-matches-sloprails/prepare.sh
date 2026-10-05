#!/usr/bin/env bash
# prepare: the ADR this check's subject names (subjects.sh: one per ADR that changed, or that links
# a rule whose folder changed; all of them when the rule runs unsplit), as
# additionalContext.subjects. Nothing that can grow is inlined: the
# ADR and each file of each linked rule are named by project path.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
load_adrs
# each lookup refuses when it fails: an empty list of ADRs or of changed paths would judge nothing
changed="$(cs '.changeset.files[].path')" || refuse "the changed paths could not be listed, so no ADR could be judged"
list="$(jq -c '.[]' <<<"$ADRS")" || refuse "the ADRs could not be listed, so none could be judged"
subjects="[]"
while IFS= read -r a; do
  [ -n "$a" ] || continue
  id="$(jq -r '.id' <<<"$a")" || refuse "an ADR's id could not be read, so it could not be judged"
  want_subject "$id" || continue
  adr_path="$(jq -r '.path' <<<"$a")" || refuse "adr/$id: its path could not be read, so it could not be judged"
  links="$(jq -r '.frontmatter.sloprails // [] | .[]' <<<"$a")" || refuse "adr/$id: its linked rules could not be read, so it could not be judged"
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
    # the other ADRs this rule enforces: a check one ADR does not decide may be another's
    others="$(jq -c --arg l "$l" --arg id "$id" '[.[] | select(.id != $id and ((.frontmatter.sloprails // []) | index($l))) | .path]' <<<"$ADRS")" ||
      refuse "adr/$id: the other ADRs linking $l could not be listed, so it could not be judged"
    for f in "$dir"/*; do
      [ -f "$f" ] || continue
      rules="$(jq -c --arg r "$l" --arg p ".sloprail/$l/$(basename "$f")" --argjson o "$others" '. + [{rule: $r, path: $p, others: $o}]' <<<"$rules")" ||
        refuse "adr/$id: the files of $l could not be listed, so it could not be judged"
    done
  done <<<"$links"
  # the rules (every file of every linked rule) go to jq through a file, not argv (Linux caps one argument at 128 KB)
  subjects="$(jq -c --arg id "$id" --arg adr "$adr_path" --slurpfile r <(printf '%s' "$rules") \
    '. + [{id: $id, adr: $adr, rules: $r[0]}]' <<<"$subjects")" || refuse "adr/$id: it could not be listed for the judge"
done <<<"$list"
n="$(jq 'length' <<<"$subjects")" || refuse "the ADRs to judge could not be counted"
if [ "$n" -eq 0 ]; then echo '{"skip": true}'; exit 0; fi
jq -c '{additionalContext: {subjects: .}}' <<<"$subjects" || refuse "the judge's input could not be built"
