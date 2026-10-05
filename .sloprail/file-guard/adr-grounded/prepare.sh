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
   | {path, status, diff, oldContent, adr: (.path | split("/")[1])}]')" ||
  refuse_error "the changed ADR files could not be listed, so none could be judged"
# each lookup refuses when it fails: a failed count read as zero would skip the judge
files="$(jq -c --arg want "$(subject_id)" '[.[] | select($want == "" or .adr == $want)]' <<<"$files")" ||
  refuse_error "the changed ADR files could not be selected, so none could be judged"
n="$(jq 'length' <<<"$files")" || refuse_error "the changed ADR files could not be counted, so none could be judged"
[ "$n" -gt 0 ] || { echo '{"skip": true}'; exit 0; }
list="$(jq -c '.[]' <<<"$files")" || refuse_error "the changed ADR files could not be listed, so none could be judged"
dir="$(mktemp -d "${TMPDIR:-/tmp}/sr-judge-adr-grounded.XXXXXX")" || refuse_error "cannot make a directory for the judge's diffs"
out="[]"
while IFS= read -r f; do
  [ -n "$f" ] || continue
  path="$(jq -r '.path' <<<"$f")" || refuse_error "a changed ADR file's path could not be read"
  key="$(printf '%s' "$path" | tr '/' '_')"
  jq -r '.diff // ""' <<<"$f" >"$dir/$key.diff" || refuse_error "$path: its diff could not be written for the judge"
  before=""
  if [[ "$path" == */ADR.md ]]; then
    jq -r '.oldContent // ""' <<<"$f" >"$dir/$key.before" || refuse_error "$path: its old content could not be written for the judge"
    [ -s "$dir/$key.before" ] && before="$dir/$key.before"
  fi
  # only the small fields of $f go to jq: its diff and old content are files, not argv (Linux caps one argument at 128 KB)
  meta="$(jq -c '{adr, path, status}' <<<"$f")" || refuse_error "$path: its fields could not be read"
  out="$(jq -c --argjson m "$meta" --arg d "$dir/$key.diff" --arg b "$before" \
    '. + [{adr: $m.adr, path: $m.path, status: $m.status, diff: $d, before: $b}]' <<<"$out")" ||
    refuse_error "$path: it could not be listed for the judge"
done <<<"$list"
subjects="$(jq -c '
  group_by(.adr)
  | map({id: .[0].adr,
         adr_before: ((map(select(.path | endswith("/ADR.md")))[0].before) // ""),
         adr_after: ((map(select((.path | endswith("/ADR.md")) and .status != "D"))[0].path) // ""),
         changes: map({path, status, diff})})' <<<"$out")" || refuse_error "the ADR subjects could not be built"
jq -c '{additionalContext: {subjects: .}}' <<<"$subjects" || refuse_error "the judge's input could not be built"
