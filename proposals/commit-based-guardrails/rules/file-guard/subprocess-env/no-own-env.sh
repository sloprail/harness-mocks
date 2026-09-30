#!/usr/bin/env bash
# adr/subprocess-env's deterministic half: outside internal/procenv, no changed file may assign
# cmd.Env. A file on the exception list may not add assignments.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
load_adrs "$(rule_qname)"
exceptions="$(jq -r '[.[] | .frontmatter.exceptions // [] | .[]] | .[]' <<<"$ADRS")"
count() { printf '%s\n' "$1" | grep -cE '\.Env[[:space:]]*=[^=]' || true; }
problems=""
while IFS= read -r f; do
  [ -n "$f" ] || continue
  path="$(jq -r '.path' <<<"$f")"
  case "$path" in internal/procenv/*) continue ;; esac
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
refuse "adr/subprocess-env (only internal/procenv builds a child process's environment):
${problems}Build the environment with internal/procenv and pass the harness's facts through its adapter."
