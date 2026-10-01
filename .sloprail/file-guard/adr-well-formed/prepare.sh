#!/usr/bin/env bash
# prepare: one subject per ADR.md added or changed, as additionalContext.subjects
# (the engine does not split subjects yet; one judge call reviews them all). The
# ADR is named by its project path and its diff is a file outside the project
# (under a temp dir); nothing that can grow is inlined.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
files="$(cs_json '[.changeset.files[] | select(.status != "D" and (.path | test("^adr/[a-z0-9-]+/ADR\\.md$"))) | {path, diff}]')"
[ "$(jq 'length' <<<"$files")" -gt 0 ] || { echo '{"skip": true}'; exit 0; }
dir="$(mktemp -d "${TMPDIR:-/tmp}/sr-judge-adr-well-formed.XXXXXX")" || refuse "cannot make a directory for the judge's diffs"
subjects="[]"
while IFS= read -r f; do
  path="$(jq -r '.path' <<<"$f")"; id="$(printf '%s' "$path" | cut -d/ -f2)"
  jq -r '.diff // ""' <<<"$f" >"$dir/$id.diff"
  subjects="$(jq -c --arg id "$id" --arg p "$path" --arg d "$dir/$id.diff" '. + [{id: $id, path: $p, diff: $d}]' <<<"$subjects")"
done < <(jq -c '.[]' <<<"$files")
jq -n -c --argjson s "$subjects" '{additionalContext: {subjects: $s}}'
