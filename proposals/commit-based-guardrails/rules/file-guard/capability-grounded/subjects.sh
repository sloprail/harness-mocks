#!/usr/bin/env bash
# One subject per capability whose file changed, or which cites a doc page that
# changed; context: the statement and, per providing harness, the text of every
# cited doc section, read from the frozen snapshot.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/snapshots.sh"
load_spec capabilities; caps="$SPEC"
changed="$(cs '.changeset.files[].path')"
subjects="[]"
while IFS= read -r c; do
  [ -n "$c" ] || continue
  id="$(jq -r '.id' <<<"$c")"
  hit=0; printf '%s\n' "$changed" | grep -Fxq "spec/capabilities/$id.yaml" && hit=1
  providers="[]"
  while IFS=$'\t' read -r h ref; do
    [ -n "$h" ] || continue
    printf '%s\n' "$changed" | grep -Fxq "$h-mock/snapshots/MANIFEST.yaml" && hit=1   # a re-frozen doc
    text="$(doc_ref_section "$h" "$ref")"
    providers="$(jq -c --arg h "$h" --arg r "$ref" --arg t "$text" '. + [{harness: $h, ref: $r, section: $t}]' <<<"$providers")"
  done < <(jq -r '.doc.providers // {} | to_entries[] | select(.value | type == "object") | .key as $h | .value.docs[] | [$h, .] | @tsv' <<<"$c")
  [ "$hit" = 1 ] || continue
  subjects="$(jq -c --arg id "$id" --argjson c "$c" --argjson p "$providers" \
    '. + [{id: $id, files: [$c.path], context: {statement: $c.doc.statement, docs: $p}}]' <<<"$subjects")"
done < <(jq -c '.[]' <<<"$caps")
# a deleted capability: judged on the words only
for p in $(cs '.changeset.files[] | select(.status == "D" and (.path | startswith("spec/capabilities/"))) | .path'); do
  subjects="$(jq -c --arg p "$p" '. + [{id: ($p | ltrimstr("spec/capabilities/") | rtrimstr(".yaml")), files: [$p], context: {removed: true}}]' <<<"$subjects")"
done
jq -n -c --argjson s "$subjects" '{subjects: $s}'
