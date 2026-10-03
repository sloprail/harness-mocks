#!/usr/bin/env bash
# prepare: the capability this check's subject names (subjects.sh: one per capability whose
# file changed, or which cites a doc page that changed; all of them when the rule runs unsplit),
# as additionalContext.subjects, with only the harnesses in
# question: those whose cell changed (cells.sh; every harness when the statement
# or the whole file changed) or whose cited doc page was re-frozen. Per capability: the statement (a short
# string) and, per providing harness, each cited doc ref with the path of its
# frozen page (a local file in the doc cache under the git dir, fetched on a miss
# and checked against the MANIFEST's sha256 by doc_copy; a page that cannot be
# had fails this check closed) and the line its cited section starts at; and
# each cited run's directory.
# A harness whose cell is {supported: false, reason, docs} is a subject too:
# its docs are listed as kind "absent", with the reason, for the judge to check
# that they really show the feature absent. A "pending" cell has nothing to
# judge: it is listed (pending) and is never coverage.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/snapshots.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/cells.sh"
load_spec capabilities; caps="$SPEC"; load_touched
changed="$(cs '.changeset.files[].path')"
subjects="[]"
while IFS= read -r c; do
  [ -n "$c" ] || continue
  id="$(jq -r '.id' <<<"$c")"
  want_subject "$id" || continue
  hs=""   # the harnesses in question, one per line ("*": all)
  printf '%s\n' "$changed" | grep -Fxq "spec/capabilities/$id.yaml" && hs="$(touched_harnesses "spec/capabilities/$id.yaml")"
  refs="$(jq -r '.doc.providers // {} | to_entries[] | select(.value | type == "object") | .key as $h
    | (if .value.supported == false then "absent" else "supports" end) as $k | .value.docs[] | [$h, $k, .] | @tsv' <<<"$c")"
  while IFS=$'\t' read -r h _ ref; do
    [ -n "$h" ] || continue
    printf '%s\n' "$changed" | grep -Fxq "$h-mock/snapshots/MANIFEST.yaml" && hs="$(printf '%s\n%s' "$hs" "$h")"   # a re-frozen doc
  done <<<"$refs"
  hs="$(printf '%s\n' "$hs" | sed '/^$/d' | sort -u)"
  [ -n "$hs" ] || continue
  # keep only the cited docs and runs of the harnesses in question
  if ! printf '%s\n' "$hs" | grep -Fxq '*'; then
    refs="$(printf '%s\n' "$refs" | awk -F'\t' -v keep="$(printf '%s ' $hs)" 'BEGIN{n=split(keep,k," "); for(i=1;i<=n;i++) want[k[i]]=1} want[$1]')"
    c="$(jq -c --argjson hs "$(printf '%s\n' "$hs" | jq -R . | jq -sc .)" '.doc.providers = ((.doc.providers // {}) | with_entries(select(.key as $k | $hs | index($k))))' <<<"$c")"
  fi
  docs="[]"
  while IFS=$'\t' read -r h kind ref; do
    [ -n "$h" ] || continue
    f="$(doc_copy "$h" "$ref")" || { echo "$DOC_ERROR" >&2; exit 1; }
    a="${ref#*#}"; [ "$a" = "$ref" ] && a=""
    line="$(awk -v a="$a" 'a != "" && /^#+ / { t = tolower($0); sub(/^#+ +/, "", t); gsub(/[^a-z0-9 -]/, "", t); gsub(/ /, "-", t); if (t == a) { print NR; exit } }' "$f")"
    docs="$(jq -c --arg h "$h" --arg k "$kind" --arg r "$ref" --arg p "$f" --arg l "${line:-0}" --arg n "$(wc -l <"$f" | tr -d ' ')" \
      '. + [{harness: $h, kind: $k, ref: $r, path: $p, line: ($l | tonumber), lines: ($n | tonumber)}]' <<<"$docs")"
  done <<<"$refs"
  # each providing harness's cited runs (project paths: the judge reads their
  # setup and samples) ground what its docs leave unsaid
  runs="$(jq -c '[.doc.providers // {} | to_entries[] | select(.value | type == "object" and .supported == null) | .key as $h | .value.runs[]? | {harness: $h, path: .}]' <<<"$c")"
  absent="$(jq -c '[.doc.providers // {} | to_entries[] | select(.value | type == "object" and .supported == false) | {harness: .key, reason: .value.reason}]' <<<"$c")"
  pending="$(jq -c '[.doc.providers // {} | to_entries[] | select(.value == "pending") | .key]' <<<"$c")"
  subjects="$(jq -c --arg id "$id" --arg st "$(jq -r '.doc.statement // ""' <<<"$c")" --argjson d "$docs" --argjson r "$runs" --argjson ab "$absent" --argjson pe "$pending" --arg p "spec/capabilities/$id.yaml" \
    '. + [{id: $id, removed: false, path: $p, statement: $st, docs: $d, runs: $r, absent: $ab, pending: $pe}]' <<<"$subjects")"
done < <(jq -c '.[]' <<<"$caps")
# a deleted capability: judged on the words only
for p in $(cs '.changeset.files[] | select(.status == "D" and (.path | startswith("spec/capabilities/"))) | .path'); do
  want_subject "$(basename "$p" .yaml)" || continue
  subjects="$(jq -c --arg p "$p" '. + [{id: ($p | ltrimstr("spec/capabilities/") | rtrimstr(".yaml")), removed: true, path: "", statement: "", docs: []}]' <<<"$subjects")"
done
if [ "$(jq 'length' <<<"$subjects")" -eq 0 ]; then echo '{"skip": true}'; exit 0; fi
jq -n -c --argjson s "$subjects" '{additionalContext: {subjects: $s}}'
