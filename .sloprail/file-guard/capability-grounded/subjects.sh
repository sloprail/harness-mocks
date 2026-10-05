#!/usr/bin/env bash
# subjects: one per capability whose file changed, or whose cited recording changed (a file under a
# run its cells cite). A deleted capability is a subject too. A doc page is NOT a trigger and NOT in
# the fingerprint: recordings own the truth, a doc is re-frozen only together with a recording, and
# a doc change alone re-judges nothing. The judge may still READ the docs (paths in prepare.sh).
#   files        spec/capabilities/<id>.yaml, and the changed files under the runs it cites
#   fingerprint  what the judge reads beyond them: the harnesses in question (a changed cell, a
#                changed recording, or all), the capability file and each cited run directory.
# A change to capability A leaves capability B's subject (files and fingerprint) as it was.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
slim_payload '^spec/capabilities/'
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/cells.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/snapshots.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/touched.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/subjects.sh"
load_spec capabilities; load_touched
arr="$(printf '%s' "$payload" | jq -c --argjson caps "$SPEC" --arg tt "$TOUCHED_TSV" '
  [.changeset.files[].path] as $changed
  | [$tt | split("\n")[] | select(length > 0) | split("\t") | {p: .[0], h: .[1]}] as $tr
  | ([$caps[] | .id as $id | "spec/capabilities/\($id).yaml" as $p
      | (.doc.providers // {} | to_entries | map(select(.value | type == "object"))) as $prov
      | ($prov | map(select(((.value.docs // []) | length) + ((.value.runs // []) | length) > 0) | .key)) as $cited
      | ($prov | map(select(any((.value.runs // [])[]; . as $r | any($changed[]; startswith($r + "/")))) | .key)) as $rerecorded
      | (if $changed | index($p) then [$tr[] | select(.p == $p) | .h] else [] end) as $cells
      | (($cells + $rerecorded) | unique) as $hs
      | select($hs | length > 0)
      | (if $hs | index("*") then $cited else $hs end) as $look
      | {id: $id,
         files: ((if $changed | index($p) then [$p] else [] end)
                 + [$prov[] | select(.key as $h | $look | index($h)) | (.value.runs // [])[] | . as $r | $changed[] | select(startswith($r + "/"))]),
         deps: ([$p] + [$look[] | . as $h | (($prov[] | select(.key == $h) | .value.runs // [])[])]),
         extra: ("harnesses:" + ($hs | join(" ")))}])
    + [.changeset.files[] | select(.status == "D" and (.path | startswith("spec/capabilities/")))
       | {id: (.path | ltrimstr("spec/capabilities/") | rtrimstr(".yaml")), files: [.path]}]')"
sub_finish unclaimed "$arr"
