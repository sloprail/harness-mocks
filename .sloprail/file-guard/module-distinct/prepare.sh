#!/usr/bin/env bash
# prepare: the module this check's subject names (subjects.sh: one per module.yaml added or
# changed), every OTHER module with its concern, home and api (one line and a few globs: small
# enough to inline), and for each of the subject's home globs the files it matches, one path per
# line in a file outside the project (nothing that can grow is put in the prompt).
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/modules.sh"
load_modules
# every lookup refuses when it fails: a failed one must not become "no module" (skip) or a module
# with no home files; and what grows (the other modules) goes to jq by file, never on its command line
dir="$(subject_id)"
if [ -z "$dir" ]; then   # unsplit: the first module.yaml of the changeset
  dir="$(cs '[.changeset.files[].path | select(endswith("/module.yaml"))][0] // "" | rtrimstr("/module.yaml")')" ||
    refuse "the changed module.yaml could not be worked out, so the module to judge is unknown"
fi
subject="$(jq -c --arg d "$dir" '[.[] | select(.dir == $d)][0] // empty' <<<"$MODULES")" || refuse "could not look up the module $dir"
[ -n "$subject" ] || { jq -n '{skip: true}'; exit 0; }
work="$(mktemp -d "${TMPDIR:-/tmp}/sr-judge-module-distinct.XXXXXX")" || refuse "cannot make a directory for the judge's files"
module_home_files "$subject" >"$work/all" || refuse "the files the home globs of $dir match could not be listed"
globs="$(jq -r '.home[]' <<<"$subject")" || refuse "could not read the home globs of $dir"
homes="[]"; i=0
while IFS= read -r g; do
  [ -n "$g" ] || continue
  i=$((i + 1))
  awk -F'\t' -v g="$g" '$1 == g {print $2}' "$work/all" | sort -u >"$work/home-$i.files" || refuse "could not list the files the home glob $g matches"
  n="$(grep -c . "$work/home-$i.files")"   # 1 (and "0") when it matches nothing
  homes="$(jq -c --arg g "$g" --arg f "$work/home-$i.files" --argjson n "${n:-0}" '. + [{glob: $g, files: $f, count: $n}]' <<<"$homes")" ||
    refuse "could not record the files the home glob $g matches"
done <<<"$globs"
jq -c --argjson h "$homes" '. + {homes: $h}' <<<"$subject" >"$work/subject.json" || refuse "could not build the module $dir for the judge"
jq -c --arg d "$dir" '[.[] | select(.dir != $d)]' <<<"$MODULES" >"$work/others.json" || refuse "could not list the other modules"
jq -n -c --slurpfile s "$work/subject.json" --slurpfile o "$work/others.json" \
  '{additionalContext: {module: $s[0], others: $o[0]}}' || refuse "could not build the judge's context"
