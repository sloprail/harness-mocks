#!/usr/bin/env bash
# subjects: one per module.yaml added or changed.
#   files        that module.yaml
#   fingerprint  what the judge compares it with: the concern, home and api of every OTHER module
#                (a change to another module judges this one again), and the files (and their
#                content) the module's own home globs match (a file landing in its home does too).
# A change to one module leaves a module whose files and surroundings did not change as it was.
# Nothing here goes on without what it looked up: a path list, a module or a home listing that
# cannot be worked out refuses (a subject silently left out is a module nobody judges). The
# values that grow with the repo (the modules, the files a home matches) go to jq by file, never
# as one command-line argument (Linux caps one at 128 KB).
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
slim_payload
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/modules.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/subjects.sh"
load_modules
work="$(mktemp -d "${TMPDIR:-/tmp}/sr-subjects-module-distinct.XXXXXX")" || refuse "cannot make a directory for the subjects' work files"
trap 'rm -rf "$work"' EXIT
printf '%s' "$MODULES" >"$work/modules"
paths="$(changed_paths)" || refuse "the changed paths could not be listed, so the modules to judge cannot be worked out"
mpaths="$(grep '/module\.yaml$' <<<"$paths" || true)"
out=""
while IFS= read -r p; do
  [ -n "$p" ] || continue
  d="$(dirname "$p")"
  m="$(jq -c --arg d "$d" '[.[] | select(.dir == $d)][0] // empty' <<<"$MODULES")" || refuse "could not look up the module in $d"
  [ -n "$m" ] || m="$(jq -nc --arg d "$d" '{dir: $d, home: []}')" || refuse "could not build the entry of the removed module $d"
  module_home_files "$m" >"$work/home" || refuse "the files $d's home globs match could not be listed, so its subject cannot be keyed"
  entry="$(jq -nc --arg p "$p" --slurpfile all "$work/modules" --argjson m "$m" --rawfile h "$work/home" \
    '{id: $m.dir, files: [$p],
      extra: ("others:" + ([$all[0][] | select(.dir != $m.dir)] | sort_by(.dir) | tojson) + "\nhome:" + ($h | sub("\n+$"; "")))}')" ||
    refuse "could not build the subject of $p"
  out="$out$entry"$'\n'
done <<<"$mpaths"
subs="$(printf '%s' "$out" | jq -sc .)" || refuse "the subjects could not be collected into one list"
sub_finish unclaimed "$subs"
