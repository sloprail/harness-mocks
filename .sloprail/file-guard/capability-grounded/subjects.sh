#!/usr/bin/env bash
# subjects: one per capability whose file changed, or which cites a doc page of a harness whose
# MANIFEST changed (the same capabilities prepare.sh judges). A deleted capability is a subject too.
#   files        spec/capabilities/<id>.yaml, and the MANIFEST of each harness whose doc it cites
#   fingerprint  what the judge reads beyond them: the harnesses in question (a changed cell, or
#                all), the capability file, and per harness in question its MANIFEST (the docs'
#                frozen hashes) and each cited run directory.
# A change to capability A leaves capability B's subject (files and fingerprint) as it was.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
slim_payload '^spec/capabilities/'
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/cells.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/subjects.sh"
load_spec capabilities; load_touched
arr="$(printf '%s' "$payload" | jq -c --argjson caps "$SPEC" --arg tt "$TOUCHED_TSV" '
  [.changeset.files[].path] as $changed
  | [$tt | split("\n")[] | select(length > 0) | split("\t") | {p: .[0], h: .[1]}] as $tr
  | ([$caps[] | .id as $id | "spec/capabilities/\($id).yaml" as $p
      | (.doc.providers // {} | to_entries | map(select(.value | type == "object"))) as $prov
      | ($prov | map(select((.value.docs // []) | length > 0) | .key)) as $cited
      | ($cited | map(select(. as $h | $changed | index("\($h)-mock/snapshots/MANIFEST.yaml")))) as $refrozen
      | (if $changed | index($p) then [$tr[] | select(.p == $p) | .h] else [] end) as $cells
      | (($cells + $refrozen) | unique) as $hs
      | select($hs | length > 0)
      | (if $hs | index("*") then $cited else $hs end) as $look
      | {id: $id,
         files: ((if $changed | index($p) then [$p] else [] end) + ($refrozen | map("\(.)-mock/snapshots/MANIFEST.yaml"))),
         deps: ([$p] + [$look[] | . as $h | "\($h)-mock/snapshots/MANIFEST.yaml", (($prov[] | select(.key == $h) | .value.runs // [])[])]),
         extra: ("harnesses:" + ($hs | join(" ")))}])
    + [.changeset.files[] | select(.status == "D" and (.path | startswith("spec/capabilities/")))
       | {id: (.path | ltrimstr("spec/capabilities/") | rtrimstr(".yaml")), files: [.path]}]')"
sub_finish unclaimed "$arr"
