#!/usr/bin/env bash
# subjects: one per capability whose file changed, or which cites a doc page whose MANIFEST entry
# changed (touched.sh: not any page of that harness; the same capabilities prepare.sh judges). A
# deleted capability is a subject too.
#   files        spec/capabilities/<id>.yaml, and the MANIFEST of each harness whose cited page changed
#   fingerprint  what the judge reads beyond them: the harnesses in question (a changed cell, or
#                all), the capability file, per harness in question the frozen hash of each page the
#                capability cites (not the whole MANIFEST), and each cited run directory.
# A change to capability A leaves capability B's subject (files and fingerprint) as it was.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
slim_payload '^(spec/capabilities/|[a-z0-9]+-mock/snapshots/MANIFEST\.yaml$)'
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/cells.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/snapshots.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/touched.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/subjects.sh"
load_spec capabilities; load_touched; load_doc_changes; load_doc_shas
arr="$(printf '%s' "$payload" | jq -c --argjson caps "$SPEC" --arg tt "$TOUCHED_TSV" --arg dc "$DOC_CHANGES_TSV" --argjson shas "$DOC_SHAS" '
  [.changeset.files[].path] as $changed
  | [$tt | split("\n")[] | select(length > 0) | split("\t") | {p: .[0], h: .[1]}] as $tr
  | [$dc | split("\n")[] | select(length > 0) | split("\t") | {h: .[0], u: .[1]}] as $dcs
  | ([$caps[] | .id as $id | "spec/capabilities/\($id).yaml" as $p
      | (.doc.providers // {} | to_entries | map(select(.value | type == "object"))) as $prov
      | ($prov | map(select(((.value.docs // []) | length) + ((.value.runs // []) | length) > 0) | .key)) as $cited
      | ($prov | map(select((.value.docs // []) | length > 0)
          | select(.key as $h | any(.value.docs[]; (split("#")[0]) as $u | any($dcs[]; .h == $h and (.u == "*" or .u == $u)))) | .key)) as $refrozen
      | (if $changed | index($p) then [$tr[] | select(.p == $p) | .h] else [] end) as $cells
      | (($cells + $refrozen) | unique) as $hs
      | select($hs | length > 0)
      | (if $hs | index("*") then $cited else $hs end) as $look
      | {id: $id,
         files: ((if $changed | index($p) then [$p] else [] end) + ($refrozen | map("\(.)-mock/snapshots/MANIFEST.yaml"))),
         deps: ([$p] + [$look[] | . as $h | (($prov[] | select(.key == $h) | .value.runs // [])[])]),
         extra: ("harnesses:" + ($hs | join(" "))
                 + "\ndocs:" + ([$look[] | . as $h | ($prov[] | select(.key == $h) | .value.docs // [])[] | (split("#")[0]) as $u | "\($h) \($u)=\($shas[$h].docs[$u] // "-")"] | unique | join(";"))
                 + "\nversions:" + ([$look[]] | unique | map(. as $h | "\($h)=\($shas[$h].version // "")") | join(";")))}])
    + [.changeset.files[] | select(.status == "D" and (.path | startswith("spec/capabilities/")))
       | {id: (.path | ltrimstr("spec/capabilities/") | rtrimstr(".yaml")), files: [.path]}]')"
sub_finish unclaimed "$arr"
