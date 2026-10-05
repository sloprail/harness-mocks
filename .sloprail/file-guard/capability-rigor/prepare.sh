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
load_spec capabilities
load_markers proves; proves="$MARKERS"
# loaded here, in this shell: a $(...) loses what a loader sets, and a refusal inside one exits only it
load_spec capabilities; load_touched
load_touched_markers || refuse "the capability markers this change touches could not be worked out, so nothing could be prepared for the judge"
subjects="[]"
# Every lookup below that fails refuses: a prepare that cannot work out what to put before the judge
# must not hand it less (or nothing, which skips the model) and let the verdict pass on that.
pairs="$(rigor_pairs)" || refuse_error "the touched capability pairs could not be worked out, so nothing could be prepared for the judge"
while IFS=$'\t' read -r pair cell c; do
  [ -n "$pair" ] || continue
  id="${pair%%/*}"; h="${pair#*/}"
  # Nothing that can grow is inlined: the judge gets paths and reads them.
  # Runs and tests are in the project; each doc is its page, a local file (the frozen
  # copy or, when the page has drifted since, the live one; the doc is read,
  # never part of the verdict's key; a page that cannot be had fails this check closed), with
  # the line its cited section starts at.
  cited_runs="$(jq -r '(.runs // [])[]' <<<"$cell")" || refuse_error "$pair: its cited runs could not be listed, so it could not be prepared for the judge"
  cited_docs="$(jq -r '(.docs // [])[]' <<<"$cell")" || refuse_error "$pair: its cited docs could not be listed, so it could not be prepared for the judge"
  runs="[]"; for r in $cited_runs; do
    samples="$(cd "$SR_TREE" && for s in "$r"/samples/*/events.jsonl; do if [ -f "$s" ]; then printf '%s\n' "$s"; fi; done | jq -R . | jq -sc .)" ||
      refuse "$pair: the samples of $r could not be listed, so it could not be prepared for the judge"
    runs="$(jq -c --arg r "$r" --slurpfile sm <(printf '%s' "$samples") \
      '. + [{name: ($r | split("/") | last), dir: $r, setup: ($r + "/setup"), samples: $sm[0]}]' <<<"$runs")" ||
      refuse_error "$pair: the run $r could not be listed, so it could not be prepared for the judge"; done
  tests="$(printf '%s\n' "$proves" | awk -F'\t' -v q="$id/$h" '$2 == q {print $1}' | sort -u | jq -R . | jq -sc 'map(select(. != ""))')" ||
    refuse_error "$pair: the tests proving it could not be listed, so it could not be prepared for the judge"
  docs="[]"; for ref in $cited_docs; do
    doc_copy_var "$h" "$ref" || { echo "${DOC_ERROR:-could not read the frozen page of $ref}" >&2; exit 1; }; f="$DOC_PATH"
    a="${ref#*#}"; [ "$a" = "$ref" ] && a=""
    line="$(awk -v a="$a" 'a != "" && /^#+ / { t = tolower($0); sub(/^#+ +/, "", t); gsub(/[^a-z0-9 -]/, "", t); gsub(/ /, "-", t); if (t == a) { print NR; exit } }' "$f")"
    docs="$(jq -c --arg r "$ref" --arg p "$f" --arg l "${line:-1}" --arg n "$(wc -l <"$f" | tr -d ' ')" \
      '. + [{ref: $r, path: $p, line: ($l | tonumber), lines: ($n | tonumber)}]' <<<"$docs")" ||
      refuse_error "$pair: the cited doc $ref could not be listed, so it could not be prepared for the judge"; done
  statement="$(jq -r '.doc.statement' <<<"$c")" || refuse_error "$pair: its statement could not be read, so it could not be prepared for the judge"
  deviations="$(jq -c '.deviations // []' <<<"$cell")" || refuse_error "$pair: its deviations could not be read, so it could not be prepared for the judge"
  # the lists go to jq through files, not the command line (one argv entry is capped at 128 KB on Linux)
  subjects="$(jq -c --arg id "$id/$h" --arg st "$statement" --arg h "$h" \
    --slurpfile docs <(printf '%s' "$docs") --slurpfile runs <(printf '%s' "$runs") --slurpfile tests <(printf '%s' "$tests") --arg path "spec/capabilities/$id.yaml" \
    --slurpfile dev <(printf '%s' "$deviations") \
    '. + [{id: $id, files: ([$path] + $tests[0]), context: {harness: $h, statement: $st, docs: $docs[0], runs: $runs[0], deviations: $dev[0], tests: $tests[0]}}]' <<<"$subjects")" ||
    refuse_error "$pair: its context could not be assembled, so it could not be prepared for the judge"
done <<<"$pairs"
# nothing touched: skip the model
n="$(jq 'length' <<<"$subjects")" || refuse_error "the prepared subjects could not be counted, so nothing could be handed to the judge"
if [ "$n" -eq 0 ]; then echo '{"skip": true}'; exit 0; fi
jq -c '{additionalContext: {subjects: .}}' <<<"$subjects" || refuse_error "the prepared subjects could not be written, so nothing could be handed to the judge"
