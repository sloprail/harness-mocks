#!/usr/bin/env bash
# prepare: the (capability, harness) pairs touched, as additionalContext.subjects: the capability file changed (every
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
    printf '%s\n' "$changed" | grep -Fxq "$h-mock/snapshots/MANIFEST.yaml" && hit=1   # a re-frozen doc
    [ "$hit" = 1 ] || continue
    d="$(snap_dir "$h")"
    runs="[]"; for r in $(jq -r '.runs[]' <<<"$cell"); do
      # every sample's normalized events, inlined (capped): what the real harness did
      samples="[]"; for s in "$SR_TREE/$r"/samples/*/; do
        [ -f "$s/events.jsonl" ] || continue
        samples="$(jq -c --arg ts "$(basename "$s")" --arg ev "$(head -c 60000 "$s/events.jsonl")" '. + [{ts: $ts, events: $ev}]' <<<"$samples")"
      done
      runs="$(jq -c --arg r "$r" --arg y "$(cat "$SR_TREE/$r/run.yaml" 2>/dev/null)" --arg setup "$(cat "$SR_TREE/$r/setup/prompt.txt" 2>/dev/null)" \
        --argjson sm "$samples" '. + [{name: ($r | split("/") | last), dir: $r, run: $y, prompt: $setup, samples: $sm}]' <<<"$runs")"; done
    tests="[]"; for f in $(printf '%s\n' "$proves" | awk -F'\t' -v q="$id/$h" '$2 == q {print $1}' | sort -u); do
      tests="$(jq -c --arg p "$f" --rawfile t "$SR_TREE/$f" '. + [{path: $p, text: $t}]' <<<"$tests")"; done
    # A section over 20k chars (a page-long table) is cut to its headings and
    # the lines naming an identifier the runs or tests mention: the rest would
    # drown the judge.
    seen="$(jq -r '([.[].samples[].events] | join("\n"))' <<<"$runs"; jq -r '.[].text' <<<"$tests")"
    docs="[]"; for ref in $(jq -r '.docs[]' <<<"$cell"); do
      t="$(doc_ref_section "$h" "$ref")"
      if [ "${#t}" -gt 20000 ]; then
        ids="$(grep -oE '\b[A-Z][A-Z0-9_]{3,}\b' <<<"$seen" | sort -u | grep -Fx -f <(grep -oE '\b[A-Z][A-Z0-9_]{3,}\b' <<<"$t" | sort -u) | paste -sd'|' -)"
        t="(section cut to its headings and the lines naming ${ids:-nothing the runs or tests mention})
$(grep -E "^#${ids:+|$ids}" <<<"$t")"
      fi
      docs="$(jq -c --arg r "$ref" --arg t "$t" '. + [{ref: $r, section: $t}]' <<<"$docs")"; done
    subjects="$(jq -c --arg id "$id/$h" --arg st "$(jq -r '.doc.statement' <<<"$c")" --arg h "$h" \
      --argjson docs "$docs" --argjson runs "$runs" --argjson tests "$tests" --arg path "spec/capabilities/$id.yaml" \
      --argjson dev "$(jq -c '.deviations // []' <<<"$cell")" \
      '. + [{id: $id, files: ([$path] + [$tests[].path]), context: {harness: $h, statement: $st, docs: $docs, runs: $runs, deviations: $dev, tests: $tests}}]' <<<"$subjects")"
  done
done < <(jq -c '.[]' <<<"$caps")
# nothing touched: skip the model
if [ "$(jq 'length' <<<"$subjects")" -eq 0 ]; then echo '{"skip": true}'; exit 0; fi
jq -n -c --argjson s "$subjects" '{additionalContext: {subjects: $s}}'
