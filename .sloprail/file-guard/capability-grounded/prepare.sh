#!/usr/bin/env bash
# prepare: the capability this check's subject names (subjects.sh: one per capability whose
# file changed, or which cites a recording that changed; all of them when the rule runs unsplit),
# as additionalContext.subjects, with only the harnesses in
# question: those whose cell changed (cells.sh; every harness when the statement
# or the whole file changed) or whose cited recording changed. A doc re-freeze is no trigger. Per capability: the statement (a short
# string) and, per providing harness, each cited doc ref with the path of its
# page (a local file in the doc cache under the git dir: the frozen copy or, when the
# page has drifted since, the live one (a doc is read here, never part of
# the verdict's key); a page that cannot be had fails this check closed) and the line its cited section starts at; and
# each cited run's directory.
# A harness whose cell is {supported: false, reason, docs?, runs?} is a subject too:
# its docs and runs are listed as kind "absent", with the reason, for the judge to check
# that they really show the feature absent (a recording outranks a doc). A "pending" cell has nothing to
# judge: it is listed (pending) and is never coverage.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/snapshots.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/cells.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/touched.sh"
load_spec capabilities; caps="$SPEC"; load_touched
changed="$(cs '.changeset.files[].path')" ||
  refuse "the changed files could not be listed, so nothing could be prepared for the judge"
subjects="[]"
# Every lookup below that fails refuses: a prepare that cannot work out what to put before the judge
# must not hand it less (or nothing) and let the verdict pass on that.
list="$(jq -c '.[]' <<<"$caps")" || refuse_error "the capability files could not be listed, so nothing could be prepared for the judge"
while IFS= read -r c; do
  [ -n "$c" ] || continue
  id="$(jq -r '.id' <<<"$c")" || refuse_error "a capability's id could not be read, so it could not be prepared for the judge"
  want_subject "$id" || continue
  hs=""   # the harnesses in question, one per line ("*": all)
  printf '%s\n' "$changed" | grep -Fxq "spec/capabilities/$id.yaml" && hs="$(touched_harnesses "spec/capabilities/$id.yaml")"
  refs="$(jq -r '.doc.providers // {} | to_entries[] | select(.value | type == "object") | .key as $h
    | (if .value.supported == false then "absent" else "supports" end) as $k | (.value.docs // [])[] | [$h, $k, .] | @tsv' <<<"$c")" ||
    refuse_error "$id: its cited docs could not be listed, so it could not be prepared for the judge"
  # a changed recording: a file under a run this capability's cell cites
  cited="$(jq -r '.doc.providers // {} | to_entries[] | select(.value | type == "object") | .key as $h | (.value.runs // [])[] | [$h, .] | @tsv' <<<"$c")" ||
    refuse_error "$id: its cited runs could not be listed, so it could not be prepared for the judge"
  while IFS=$'\t' read -r h r; do
    [ -n "$h" ] || continue
    printf '%s\n' "$changed" | awk -v r="$r/" 'index($0, r) == 1 {f = 1} END {exit !f}' && hs="$(printf '%s\n%s' "$hs" "$h")"
  done <<<"$cited"
  hs="$(printf '%s\n' "$hs" | sed '/^$/d' | sort -u)"
  [ -n "$hs" ] || continue
  # keep only the cited docs and runs of the harnesses in question
  if ! printf '%s\n' "$hs" | grep -Fxq '*'; then
    refs="$(printf '%s\n' "$refs" | awk -F'\t' -v keep="$(printf '%s ' $hs)" 'BEGIN{n=split(keep,k," "); for(i=1;i<=n;i++) want[k[i]]=1} want[$1]')"
    c="$(jq -c --slurpfile hs <(printf '%s\n' "$hs" | jq -R . | jq -sc .) '.doc.providers = ((.doc.providers // {}) | with_entries(select(.key as $k | $hs[0] | index($k))))' <<<"$c")" ||
      refuse_error "$id: its harnesses in question could not be narrowed, so it could not be prepared for the judge"
  fi
  docs="[]"
  while IFS=$'\t' read -r h kind ref; do
    [ -n "$h" ] || continue
    doc_copy_var "$h" "$ref" || { echo "${DOC_ERROR:-could not read the frozen page of $ref}" >&2; exit 1; }; f="$DOC_PATH"
    a="${ref#*#}"; [ "$a" = "$ref" ] && a=""
    line="$(awk -v a="$a" 'a != "" && /^#+ / { t = tolower($0); sub(/^#+ +/, "", t); gsub(/[^a-z0-9 -]/, "", t); gsub(/ /, "-", t); if (t == a) { print NR; exit } }' "$f")"
    docs="$(jq -c --arg h "$h" --arg k "$kind" --arg r "$ref" --arg p "$f" --arg l "${line:-0}" --arg n "$(wc -l <"$f" | tr -d ' ')" \
      '. + [{harness: $h, kind: $k, ref: $r, path: $p, line: ($l | tonumber), lines: ($n | tonumber)}]' <<<"$docs")" ||
      refuse_error "$id: the cited doc $ref could not be listed, so it could not be prepared for the judge"
  done <<<"$refs"
  # each providing harness's cited runs (project paths: the judge reads their
  # setup and samples) ground what its docs leave unsaid
  runs="$(jq -c '[.doc.providers // {} | to_entries[] | select(.value | type == "object") | .key as $h | (if .value.supported == false then "absent" else "supports" end) as $k | .value.runs[]? | {harness: $h, kind: $k, path: .}]' <<<"$c")" ||
    refuse_error "$id: its cited runs could not be read, so it could not be prepared for the judge"
  absent="$(jq -c '[.doc.providers // {} | to_entries[] | select(.value | type == "object" and .supported == false) | {harness: .key, reason: .value.reason}]' <<<"$c")" ||
    refuse_error "$id: its unsupported cells could not be read, so it could not be prepared for the judge"
  pending="$(jq -c '[.doc.providers // {} | to_entries[] | select(.value == "pending") | .key]' <<<"$c")" ||
    refuse_error "$id: its pending cells could not be read, so it could not be prepared for the judge"
  statement="$(jq -r '.doc.statement // ""' <<<"$c")" || refuse_error "$id: its statement could not be read, so it could not be prepared for the judge"
  # the lists go to jq through files, not the command line (one argv entry is capped at 128 KB on Linux)
  subjects="$(jq -c --arg id "$id" --arg st "$statement" --slurpfile d <(printf '%s' "$docs") --slurpfile r <(printf '%s' "$runs") --slurpfile ab <(printf '%s' "$absent") --slurpfile pe <(printf '%s' "$pending") --arg p "spec/capabilities/$id.yaml" \
    '. + [{id: $id, removed: false, path: $p, statement: $st, docs: $d[0], runs: $r[0], absent: $ab[0], pending: $pe[0]}]' <<<"$subjects")" ||
    refuse_error "$id: its context could not be assembled, so it could not be prepared for the judge"
done <<<"$list"
# a deleted capability: judged on the words only
deleted="$(cs '.changeset.files[] | select(.status == "D" and (.path | startswith("spec/capabilities/"))) | .path')" ||
  refuse_error "the deleted capability files could not be listed, so nothing could be prepared for the judge"
for p in $deleted; do
  want_subject "$(basename "$p" .yaml)" || continue
  subjects="$(jq -c --arg p "$p" '. + [{id: ($p | ltrimstr("spec/capabilities/") | rtrimstr(".yaml")), removed: true, path: "", statement: "", docs: []}]' <<<"$subjects")" ||
    refuse_error "$p: the deleted capability could not be listed, so it could not be prepared for the judge"
done
n="$(jq 'length' <<<"$subjects")" || refuse_error "the prepared subjects could not be counted, so nothing could be handed to the judge"
if [ "$n" -eq 0 ]; then echo '{"skip": true}'; exit 0; fi
jq -c '{additionalContext: {subjects: .}}' <<<"$subjects" || refuse_error "the prepared subjects could not be written, so nothing could be handed to the judge"
