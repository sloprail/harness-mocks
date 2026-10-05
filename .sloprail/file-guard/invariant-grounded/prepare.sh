#!/usr/bin/env bash
# prepare: the spec/invariants/<id>.yaml this check's subject names (subjects.sh: one per file
# added, changed or removed; all of them when the rule runs unsplit), as
# additionalContext.subjects. Nothing that can grow is inlined: `after` is the
# file's project path (absent when removed), `before` a file outside the project
# holding the old content (absent when added).
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
# each lookup refuses when it fails: a failed count read as zero would skip the judge
files="$(cs_json '[.changeset.files[] | select(.path | test("^spec/invariants/[a-z0-9-]+\\.yaml$")) | {path, status, oldContent}]')" ||
  refuse "the changed invariant files could not be listed, so none could be judged"
n="$(jq 'length' <<<"$files")" || refuse "the changed invariant files could not be counted, so none could be judged"
[ "$n" -gt 0 ] || { echo '{"skip": true}'; exit 0; }
list="$(jq -c '.[]' <<<"$files")" || refuse "the changed invariant files could not be listed, so none could be judged"
dir="$(mktemp -d "${TMPDIR:-/tmp}/sr-judge-invariant-grounded.XXXXXX")" || refuse "cannot make a directory for the judge's files"
subjects="[]"
while IFS= read -r f; do
  [ -n "$f" ] || continue
  path="$(jq -r '.path' <<<"$f")" || refuse "a changed invariant file's path could not be read"
  id="$(basename "$path" .yaml)"
  want_subject "$id" || continue
  status="$(jq -r '.status' <<<"$f")" || refuse "$path: its status could not be read"
  jq -r '.oldContent // ""' <<<"$f" >"$dir/$id.before.yaml" || refuse "$path: its old content could not be written for the judge"
  before=""; [ -s "$dir/$id.before.yaml" ] && before="$dir/$id.before.yaml"
  after="$path"; [ "$status" = "D" ] && after=""
  subjects="$(jq -c --arg id "$id" --arg st "$status" --arg b "$before" --arg a "$after" \
    '. + [{id: $id, status: $st, before: $b, after: $a}]' <<<"$subjects")" || refuse "$path: it could not be listed for the judge"
done <<<"$list"
n="$(jq 'length' <<<"$subjects")" || refuse "the invariants to judge could not be counted"
[ "$n" -gt 0 ] || { echo '{"skip": true}'; exit 0; }
jq -c '{additionalContext: {subjects: .}}' <<<"$subjects" || refuse "the judge's input could not be built"
