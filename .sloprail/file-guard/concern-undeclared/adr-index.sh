#!/usr/bin/env bash
# prepare: the index of decided concerns (every ADR's and every module's one-line
# concern; a module's concern is decided too, in its module.yaml), about 50 tokens
# an ADR where the full texts would be several hundred each, and every changed
# file with its diff written to a file outside the project (under a temp dir) for
# the judge to read. Nothing that can grow is put in the prompt.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/modules.sh"
load_adrs; load_modules
# a failed load or lookup must refuse, never read as "no ADRs", "no files" or "nothing changed"; and
# the ADRs and modules (they grow with the repo) go to jq by file, never on its command line
jq -e 'type == "array"' <<<"$ADRS" >/dev/null 2>&1 || refuse_error "the ADRs could not be loaded, so the index of decided concerns cannot be built"
nfiles="$(cs '.changeset.files | length')" || refuse_error "the changed files could not be counted, so the changeset cannot be judged"
case "$nfiles" in
  0) jq -n '{skip: true}'; exit 0 ;;
  ""|*[!0-9]*) refuse_error "the number of changed files is '$nfiles', not a count, so the changeset cannot be judged" ;;
esac
dir="$(mktemp -d "${TMPDIR:-/tmp}/sr-judge-diffs.XXXXXX")" || refuse_error "cannot make a directory for the judge's diffs"
work="$(mktemp -d "${TMPDIR:-/tmp}/sr-adr-index.XXXXXX")" || refuse_error "cannot make a directory for the index's work files"
trap 'rm -rf "$work"' EXIT
printf '%s' "$ADRS" >"$work/adrs.json"; printf '%s' "$MODULES" >"$work/modules.json"
flist="$(cs_json '.changeset.files[]')" || refuse_error "the changed files could not be listed, so the changeset cannot be judged"
files="[]"
while IFS= read -r f; do
  [ -n "$f" ] || continue
  path="$(jq -r '.path' <<<"$f")" || refuse_error "could not read a changed file's path"
  status="$(jq -r '.status' <<<"$f")" || refuse_error "could not read the status of $path"
  out="$dir/$(printf '%s' "$path" | tr '/' '_').diff"
  jq -r '.diff // ""' <<<"$f" >"$out" || refuse_error "could not write the diff of $path for the judge"
  files="$(jq -c --arg p "$path" --arg s "$status" --arg d "$out" \
    '. + [{path: $p, status: $s, diff: $d}]' <<<"$files")" || refuse_error "could not record the diff of $path for the judge"
done <<<"$flist"
printf '%s' "$files" >"$work/files.json"
jq -n -c --slurpfile a "$work/adrs.json" --slurpfile m "$work/modules.json" --slurpfile f "$work/files.json" \
  '{additionalContext: {adrs: ([$a[0][] | {id: ("adr/" + .id), concern: (.frontmatter.concern // "")}] + [$m[0][] | {id: ("module " + .dir), concern}]), files: $f[0]}}' ||
  refuse_error "could not build the judge's context"
