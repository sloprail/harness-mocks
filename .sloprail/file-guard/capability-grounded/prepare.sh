#!/usr/bin/env bash
# prepare: the capabilities whose file changed, or which cite a doc page that
# changed, as additionalContext.subjects (the engine does not split subjects
# yet; one judge call reviews them all). Per capability: the statement (a short
# string) and, per providing harness, each cited doc ref with the path of its
# frozen page (a local file in the doc cache under the git dir, fetched on a miss
# and checked against the MANIFEST's sha256 by doc_copy; a page that cannot be
# had fails this check closed) and the line its cited section starts at; and
# each cited run's directory.
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
  refs="$(jq -r '.doc.providers // {} | to_entries[] | select(.value | type == "object") | .key as $h | .value.docs[] | [$h, .] | @tsv' <<<"$c")"
  while IFS=$'\t' read -r h ref; do
    [ -n "$h" ] || continue
    printf '%s\n' "$changed" | grep -Fxq "$h-mock/snapshots/MANIFEST.yaml" && hit=1   # a re-frozen doc
  done <<<"$refs"
  [ "$hit" = 1 ] || continue
  docs="[]"
  while IFS=$'\t' read -r h ref; do
    [ -n "$h" ] || continue
    f="$(doc_copy "$h" "$ref")" || { echo "$DOC_ERROR" >&2; exit 1; }
    a="${ref#*#}"; [ "$a" = "$ref" ] && a=""
    line="$(awk -v a="$a" 'a != "" && /^#+ / { t = tolower($0); sub(/^#+ +/, "", t); gsub(/[^a-z0-9 -]/, "", t); gsub(/ /, "-", t); if (t == a) { print NR; exit } }' "$f")"
    docs="$(jq -c --arg h "$h" --arg r "$ref" --arg p "$f" --arg l "${line:-0}" --arg n "$(wc -l <"$f" | tr -d ' ')" \
      '. + [{harness: $h, ref: $r, path: $p, line: ($l | tonumber), lines: ($n | tonumber)}]' <<<"$docs")"
  done <<<"$refs"
  # each providing harness's cited runs (project paths: the judge reads their
  # setup and samples) ground what its docs leave unsaid
  runs="$(jq -c '[.doc.providers // {} | to_entries[] | select(.value | type == "object") | .key as $h | .value.runs[]? | {harness: $h, path: .}]' <<<"$c")"
  subjects="$(jq -c --arg id "$id" --arg st "$(jq -r '.doc.statement // ""' <<<"$c")" --argjson d "$docs" --argjson r "$runs" --arg p "spec/capabilities/$id.yaml" \
    '. + [{id: $id, removed: false, path: $p, statement: $st, docs: $d, runs: $r}]' <<<"$subjects")"
done < <(jq -c '.[]' <<<"$caps")
# a deleted capability: judged on the words only
for p in $(cs '.changeset.files[] | select(.status == "D" and (.path | startswith("spec/capabilities/"))) | .path'); do
  subjects="$(jq -c --arg p "$p" '. + [{id: ($p | ltrimstr("spec/capabilities/") | rtrimstr(".yaml")), removed: true, path: "", statement: "", docs: []}]' <<<"$subjects")"
done
if [ "$(jq 'length' <<<"$subjects")" -eq 0 ]; then echo '{"skip": true}'; exit 0; fi
jq -n -c --argjson s "$subjects" '{additionalContext: {subjects: $s}}'
