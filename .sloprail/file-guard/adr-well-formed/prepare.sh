#!/usr/bin/env bash
# prepare: one subject per ADR.md added or changed, as additionalContext.subjects
# (the engine does not split subjects yet; one judge call reviews them all). The
# ADR is named by its project path and its diff is a file outside the project
# (under a temp dir); nothing that can grow is inlined.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
# each lookup refuses when it fails: a failed count read as zero would skip the judge
files="$(cs_json '[.changeset.files[] | select(.status != "D" and (.path | test("^adr/[a-z0-9-]+/ADR\\.md$"))) | {path, diff}]')" ||
  refuse_error "the changed ADRs could not be listed, so none could be judged"
n="$(jq 'length' <<<"$files")" || refuse_error "the changed ADRs could not be counted, so none could be judged"
[ "$n" -gt 0 ] || { echo '{"skip": true}'; exit 0; }
list="$(jq -c '.[]' <<<"$files")" || refuse_error "the changed ADRs could not be listed, so none could be judged"
dir="$(mktemp -d "${TMPDIR:-/tmp}/sr-judge-adr-well-formed.XXXXXX")" || refuse_error "cannot make a directory for the judge's diffs"
subjects="[]"
while IFS= read -r f; do
  [ -n "$f" ] || continue
  path="$(jq -r '.path' <<<"$f")" || refuse_error "a changed ADR's path could not be read"
  id="$(printf '%s' "$path" | cut -d/ -f2)"
  jq -r '.diff // ""' <<<"$f" >"$dir/$id.diff" || refuse_error "$path: its diff could not be written for the judge"
  subjects="$(jq -c --arg id "$id" --arg p "$path" --arg d "$dir/$id.diff" '. + [{id: $id, path: $p, diff: $d}]' <<<"$subjects")" ||
    refuse_error "$path: it could not be listed for the judge"
done <<<"$list"
jq -c '{additionalContext: {subjects: .}}' <<<"$subjects" || refuse_error "the judge's input could not be built"
