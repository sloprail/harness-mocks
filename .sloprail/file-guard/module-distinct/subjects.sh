#!/usr/bin/env bash
# subjects: one per module.yaml added or changed.
#   files        that module.yaml
#   fingerprint  what the judge compares it with: the concern, home and api of every OTHER module
#                (a change to another module judges this one again), and the files (and their
#                content) the module's own home globs match (a file landing in its home does too).
# A change to one module leaves a module whose files and surroundings did not change as it was.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
slim_payload
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/modules.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/subjects.sh"
load_modules
out=""
for p in $(changed_paths | grep '/module\.yaml$' || true); do
  m="$(jq -c --arg d "$(dirname "$p")" '[.[] | select(.dir == $d)][0] // empty' <<<"$MODULES")"
  [ -n "$m" ] || m="$(jq -nc --arg d "$(dirname "$p")" '{dir: $d, home: []}')"
  out="$out$(jq -nc --arg p "$p" --argjson all "$MODULES" --argjson m "$m" --arg h "$(module_home_files "$m")" \
    '{id: $m.dir, files: [$p],
      extra: ("others:" + ([$all[] | select(.dir != $m.dir)] | sort_by(.dir) | tojson) + "\nhome:" + $h)}')"$'\n'
done
sub_finish unclaimed "$(printf '%s' "$out" | jq -sc .)"
