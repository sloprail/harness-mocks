#!/usr/bin/env bash
# ADR-0003's deterministic half: outside core/procenv, no changed file may assign
# cmd.Env. A file on the exception list may not add assignments.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
here="$(cd "$(dirname "$0")" && pwd)"
exceptions="$(yq --front-matter=extract -o=json '.exceptions // []' "$here/ADR.md" | jq -r '.[]')" ||
  refuse "ADR-0003's ADR.md could not be read"
count() { printf '%s\n' "$1" | grep -cE '\.Env[[:space:]]*=[^=]' || true; }
problems=""
while IFS= read -r f; do
  [ -n "$f" ] || continue
  path="$(jq -r '.path' <<<"$f")"
  case "$path" in core/procenv/*) continue ;; esac
  n="$(count "$(jq -r '.newContent // ""' <<<"$f")")"
  [ "$n" -gt 0 ] || continue
  if printf '%s\n' "$exceptions" | grep -Fxq -- "$path"; then
    o="$(count "$(jq -r '.oldContent // ""' <<<"$f")")"
    [ "$n" -le "$o" ] || problems="${problems}- $path adds a cmd.Env assignment ($o → $n); it is a legacy site and may only lose them"$'\n'
  else
    problems="${problems}- $path assigns cmd.Env"$'\n'
  fi
done < <(cs_json '.changeset.files[] | select(.status != "D")' | jq -c '.')
[ -z "$problems" ] && exit 0
refuse "ADR-0003 (only core/procenv builds a child process's environment):
${problems}Build the environment with core/procenv and pass the harness's facts through its adapter."
