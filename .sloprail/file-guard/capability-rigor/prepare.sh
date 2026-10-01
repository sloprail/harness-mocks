#!/usr/bin/env bash
# prepare: the (capability, harness) pairs touched (pairs-lib.sh), as
# additionalContext.subjects. Context: statement, cited doc sections, cited runs
# (run.yaml + sample names; the judge reads samples from the project) and the
# full text of every test proving <id>/<h>.
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
  # Docs are not inlined: a cited section can be huge, or not the only place
  # the page describes the behaviour. The judge gets each frozen page as a
  # local file (fetched on a miss and checked against the MANIFEST's sha256
  # by doc_copy; a page that cannot be had fails this check closed) and the
  # line its cited section starts at.
  seen="$(jq -r '([.[].samples[].events] | join("\n"))' <<<"$runs"; jq -r '.[].text' <<<"$tests")"
  docs="[]"; for ref in $(jq -r '.docs[]' <<<"$cell"); do
    f="$(doc_copy "$h" "$ref")" || { echo "$DOC_ERROR" >&2; exit 1; }
    a="${ref#*#}"; [ "$a" = "$ref" ] && a=""
    line="$(awk -v a="$a" 'a != "" && /^#+ / { t = tolower($0); sub(/^#+ +/, "", t); gsub(/[^a-z0-9 -]/, "", t); gsub(/ /, "-", t); if (t == a) { print NR; exit } }' "$f")"
    # and an excerpt the judge always sees, whether or not it opens the page
    # (measured: it often does not): the page's lines naming an identifier the
    # runs or tests mention. A floor, never the whole story.
    ids="$(grep -oE '\b[A-Z][A-Z0-9_]{3,}\b' <<<"$seen" | sort -u | grep -Fx -f <(grep -oE '\b[A-Z][A-Z0-9_]{3,}\b' "$f" | sort -u) | paste -sd'|' -)"
    ex="$([ -n "$ids" ] && grep -nwE "$ids" "$f" | head -c 20000)"
    docs="$(jq -c --arg r "$ref" --arg p "$f" --arg l "${line:-1}" --arg n "$(wc -l <"$f" | tr -d ' ')" --arg ex "$ex" \
      '. + [{ref: $r, path: $p, line: ($l | tonumber), lines: ($n | tonumber), excerpt: $ex}]' <<<"$docs")"; done
  subjects="$(jq -c --arg id "$id/$h" --arg st "$(jq -r '.doc.statement' <<<"$c")" --arg h "$h" \
    --argjson docs "$docs" --argjson runs "$runs" --argjson tests "$tests" --arg path "spec/capabilities/$id.yaml" \
    --argjson dev "$(jq -c '.deviations // []' <<<"$cell")" \
    '. + [{id: $id, files: ([$path] + [$tests[].path]), context: {harness: $h, statement: $st, docs: $docs, runs: $runs, deviations: $dev, tests: $tests}}]' <<<"$subjects")"
done < <(rigor_pairs)
# nothing touched: skip the model
if [ "$(jq 'length' <<<"$subjects")" -eq 0 ]; then echo '{"skip": true}'; exit 0; fi
jq -n -c --argjson s "$subjects" '{additionalContext: {subjects: $s}}'
