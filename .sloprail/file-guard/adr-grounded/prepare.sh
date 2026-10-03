#!/usr/bin/env bash
# prepare: the ADR folder this check's subject names (subjects.sh: one per ADR the changeset
# touched; all of them when the rule runs unsplit), as additionalContext.subjects. Nothing that
# can grow is inlined: the ADR.md after is its project path, the ADR.md before and every diff are
# files outside the project (written under a temp dir) that the judge reads.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
files="$(cs_json '
  [.changeset.files[] | select(.path | test("^adr/[a-z0-9-]+/"))
   | {path, status, diff, oldContent, adr: (.path | split("/")[1])}]')"
files="$(jq -c --arg want "$(subject_id)" '[.[] | select($want == "" or .adr == $want)]' <<<"$files")"
[ "$(jq 'length' <<<"$files")" -gt 0 ] || { echo '{"skip": true}'; exit 0; }
dir="$(mktemp -d "${TMPDIR:-/tmp}/sr-judge-adr-grounded.XXXXXX")" || refuse "cannot make a directory for the judge's diffs"
out="[]"
while IFS= read -r f; do
  path="$(jq -r '.path' <<<"$f")"; key="$(printf '%s' "$path" | tr '/' '_')"
  jq -r '.diff // ""' <<<"$f" >"$dir/$key.diff"
  before=""
  if [[ "$path" == */ADR.md ]]; then
    jq -r '.oldContent // ""' <<<"$f" >"$dir/$key.before"
    [ -s "$dir/$key.before" ] && before="$dir/$key.before"
  fi
  out="$(jq -c --argjson f "$f" --arg d "$dir/$key.diff" --arg b "$before" \
    '. + [{adr: $f.adr, path: $f.path, status: $f.status, diff: $d, before: $b}]' <<<"$out")"
done < <(jq -c '.[]' <<<"$files")
subjects="$(jq -c '
  group_by(.adr)
  | map({id: .[0].adr,
         adr_before: ((map(select(.path | endswith("/ADR.md")))[0].before) // ""),
         adr_after: ((map(select((.path | endswith("/ADR.md")) and .status != "D"))[0].path) // ""),
         changes: map({path, status, diff})})' <<<"$out")"
jq -n -c --argjson s "$subjects" '{additionalContext: {subjects: $s}}'
