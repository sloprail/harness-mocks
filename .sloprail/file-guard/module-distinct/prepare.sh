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
dir="$(subject_id)"
if [ -z "$dir" ]; then   # unsplit: the first module.yaml of the changeset
  dir="$(cs '[.changeset.files[].path | select(endswith("/module.yaml"))][0] // "" | rtrimstr("/module.yaml")')"
fi
subject="$(jq -c --arg d "$dir" '[.[] | select(.dir == $d)][0] // empty' <<<"$MODULES")"
[ -n "$subject" ] || { jq -n '{skip: true}'; exit 0; }
work="$(mktemp -d "${TMPDIR:-/tmp}/sr-judge-module-distinct.XXXXXX")" || refuse "cannot make a directory for the judge's files"
module_home_files "$subject" >"$work/all"
homes="[]"; i=0
while IFS= read -r g; do
  i=$((i + 1))
  awk -F'\t' -v g="$g" '$1 == g {print $2}' "$work/all" | sort -u >"$work/home-$i.files"
  homes="$(jq -c --arg g "$g" --arg f "$work/home-$i.files" --argjson n "$(grep -c . "$work/home-$i.files")" '. + [{glob: $g, files: $f, count: $n}]' <<<"$homes")"
done < <(jq -r '.home[]' <<<"$subject")
jq -n -c --argjson s "$(jq -c --argjson h "$homes" '. + {homes: $h}' <<<"$subject")" --argjson o "$(jq -c --arg d "$dir" '[.[] | select(.dir != $d)]' <<<"$MODULES")" \
  '{additionalContext: {module: $s, others: $o}}'
