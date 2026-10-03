#!/usr/bin/env bash
# prepare: the spec/invariants/<id>.yaml this check's subject names (subjects.sh: one per file
# added, changed or removed; all of them when the rule runs unsplit), as
# additionalContext.subjects. Nothing that can grow is inlined: `after` is the
# file's project path (absent when removed), `before` a file outside the project
# holding the old content (absent when added).
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
files="$(cs_json '[.changeset.files[] | select(.path | test("^spec/invariants/[a-z0-9-]+\\.yaml$")) | {path, status, oldContent}]')"
[ "$(jq 'length' <<<"$files")" -gt 0 ] || { echo '{"skip": true}'; exit 0; }
dir="$(mktemp -d "${TMPDIR:-/tmp}/sr-judge-invariant-grounded.XXXXXX")" || refuse "cannot make a directory for the judge's files"
subjects="[]"
while IFS= read -r f; do
  path="$(jq -r '.path' <<<"$f")"; id="$(basename "$path" .yaml)"
  want_subject "$id" || continue
  jq -r '.oldContent // ""' <<<"$f" >"$dir/$id.before.yaml"
  before=""; [ -s "$dir/$id.before.yaml" ] && before="$dir/$id.before.yaml"
  after="$path"; [ "$(jq -r '.status' <<<"$f")" = "D" ] && after=""
  subjects="$(jq -c --arg id "$id" --arg st "$(jq -r '.status' <<<"$f")" --arg b "$before" --arg a "$after" \
    '. + [{id: $id, status: $st, before: $b, after: $a}]' <<<"$subjects")"
done < <(jq -c '.[]' <<<"$files")
[ "$(jq 'length' <<<"$subjects")" -gt 0 ] || { echo '{"skip": true}'; exit 0; }
jq -n -c --argjson s "$subjects" '{additionalContext: {subjects: $s}}'
