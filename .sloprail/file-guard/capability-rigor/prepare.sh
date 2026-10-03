#!/usr/bin/env bash
# prepare: the (capability, harness) pairs touched (pairs-lib.sh; of the subject's capability), as
# additionalContext.subjects. Context: statement, cited doc sections, cited runs
# (run.yaml + sample names; the judge reads samples from the project) and the
# path of every test proving <id>/<h>.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/snapshots.sh"
. "${SR_GUARDRAIL_DIR:-.}/pairs-lib.sh"
load_markers proves; proves="$MARKERS"
subjects="[]"
while IFS=$'\t' read -r pair cell c; do
  [ -n "$pair" ] || continue
  id="${pair%%/*}"; h="${pair#*/}"
  # Nothing that can grow is inlined: the judge gets paths and reads them.
  # Runs and tests are in the project; each doc is its frozen page, a local file
  # (fetched on a miss and checked against the MANIFEST's sha256 by doc_copy; a
  # page that cannot be had fails this check closed), with the line its cited
  # section starts at.
  runs="[]"; for r in $(jq -r '.runs[]' <<<"$cell"); do
    samples="$(cd "$SR_TREE" && for s in "$r"/samples/*/events.jsonl; do [ -f "$s" ] && printf '%s\n' "$s"; done | jq -R . | jq -sc .)"
    runs="$(jq -c --arg r "$r" --argjson sm "$samples" \
      '. + [{name: ($r | split("/") | last), dir: $r, setup: ($r + "/setup"), samples: $sm}]' <<<"$runs")"; done
  tests="$(printf '%s\n' "$proves" | awk -F'\t' -v q="$id/$h" '$2 == q {print $1}' | sort -u | jq -R . | jq -sc 'map(select(. != ""))')"
  docs="[]"; for ref in $(jq -r '.docs[]' <<<"$cell"); do
    f="$(doc_copy "$h" "$ref")" || { echo "$DOC_ERROR" >&2; exit 1; }
    a="${ref#*#}"; [ "$a" = "$ref" ] && a=""
    line="$(awk -v a="$a" 'a != "" && /^#+ / { t = tolower($0); sub(/^#+ +/, "", t); gsub(/[^a-z0-9 -]/, "", t); gsub(/ /, "-", t); if (t == a) { print NR; exit } }' "$f")"
    docs="$(jq -c --arg r "$ref" --arg p "$f" --arg l "${line:-1}" --arg n "$(wc -l <"$f" | tr -d ' ')" \
      '. + [{ref: $r, path: $p, line: ($l | tonumber), lines: ($n | tonumber)}]' <<<"$docs")"; done
  subjects="$(jq -c --arg id "$id/$h" --arg st "$(jq -r '.doc.statement' <<<"$c")" --arg h "$h" \
    --argjson docs "$docs" --argjson runs "$runs" --argjson tests "$tests" --arg path "spec/capabilities/$id.yaml" \
    --argjson dev "$(jq -c '.deviations // []' <<<"$cell")" \
    '. + [{id: $id, files: ([$path] + $tests), context: {harness: $h, statement: $st, docs: $docs, runs: $runs, deviations: $dev, tests: $tests}}]' <<<"$subjects")"
done < <(rigor_pairs)
# nothing touched: skip the model
if [ "$(jq 'length' <<<"$subjects")" -eq 0 ]; then echo '{"skip": true}'; exit 0; fi
jq -n -c --argjson s "$subjects" '{additionalContext: {subjects: $s}}'
