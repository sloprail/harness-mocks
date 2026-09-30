#!/usr/bin/env bash
# (capability, harness) pairs touched: the capability file changed (every
# providing harness); a marker naming it changed (sr:capability → every
# harness; sr:provides/sr:proves <id>/<h> → that harness); or a snapshot it
# cites for <h> changed. Context: statement, cited doc sections, cited runs
# (run.yaml + sample names; the judge reads samples from the project) and the
# full text of every test proving <id>/<h>.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/snapshots.sh"
load_spec capabilities; caps="$SPEC"
load_markers proves; proves="$MARKERS"
changed="$(cs '.changeset.files[].path')"
fqns="$(cs '.changeset.files[] | ((.newMarkers // []) + (.oldMarkers // []))[] | select(.kind == "capability" or .kind == "provides" or .kind == "proves") | .fqn')"
subjects="[]"
while IFS= read -r c; do
  [ -n "$c" ] || continue
  id="$(jq -r '.id' <<<"$c")"
  for h in $(jq -r '.doc.providers // {} | to_entries[] | select(.value | type == "object") | .key' <<<"$c"); do
    cell="$(jq -c --arg h "$h" '.doc.providers[$h]' <<<"$c")"
    hit=0
    printf '%s\n' "$changed" | grep -Fxq "spec/capabilities/$id.yaml" && hit=1
    printf '%s\n' "$fqns" | grep -Fxq -e "$id" -e "$id/$h" && hit=1
    for r in $(jq -r '.runs[]' <<<"$cell"); do printf '%s\n' "$changed" | grep -q "^$r/" && hit=1; done
    for u in $(jq -r '.docs[]' <<<"$cell"); do f="$(doc_file "$h" "$u")" && printf '%s\n' "$changed" | grep -Fxq "$h-mock/snapshots/$f" && hit=1; done
    [ "$hit" = 1 ] || continue
    d="$(snap_dir "$h")"
    docs="[]"; for ref in $(jq -r '.docs[]' <<<"$cell"); do
      docs="$(jq -c --arg r "$ref" --arg t "$(doc_ref_section "$h" "$ref")" '. + [{ref: $r, section: $t}]' <<<"$docs")"; done
    runs="[]"; for r in $(jq -r '.runs[]' <<<"$cell"); do
      runs="$(jq -c --arg r "$r" --arg y "$(cat "$SR_TREE/$r/run.yaml" 2>/dev/null)" --arg s "$(ls "$SR_TREE/$r/samples" 2>/dev/null | tr '\n' ' ')" \
        '. + [{name: ($r | split("/") | last), dir: $r, run: $y, samples: $s}]' <<<"$runs")"; done
    tests="[]"; for f in $(printf '%s\n' "$proves" | awk -F'\t' -v q="$id/$h" '$2 == q {print $1}' | sort -u); do
      tests="$(jq -c --arg p "$f" --rawfile t "$SR_TREE/$f" '. + [{path: $p, text: $t}]' <<<"$tests")"; done
    subjects="$(jq -c --arg id "$id/$h" --arg st "$(jq -r '.doc.statement' <<<"$c")" --arg h "$h" \
      --argjson docs "$docs" --argjson runs "$runs" --argjson tests "$tests" --arg path "spec/capabilities/$id.yaml" \
      '. + [{id: $id, files: ([$path] + [$tests[].path]), context: {harness: $h, statement: $st, docs: $docs, runs: $runs, tests: $tests}}]' <<<"$subjects")"
  done
done < <(jq -c '.[]' <<<"$caps")
jq -n -c --argjson s "$subjects" '{subjects: $s}'
