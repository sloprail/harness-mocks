#!/usr/bin/env bash
# prepare: steps 1 and 2 of module-leaks. Emits {"skip": true} when nothing is
# left to judge, else the leftover candidates grouped by module, with the text
# of the ADRs linking each module (frontmatter `modules:`).
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/modules.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
load_modules; load_adrs
added="$(added_lines | awk -F'\t' 'NF >= 2 {print $1 ":" $2}')"      # path:line of every added line
changed="$(cs '.changeset.files[].path')"
work="$(mktemp -d "${TMPDIR:-/tmp}/module-leaks.XXXXXX")"; trap 'rm -rf "$work"' EXIT

groups="[]"
while IFS= read -r m; do
  [ -n "$m" ] || continue
  dir="$(jq -r '.dir' <<<"$m")"
  [ -x "$SR_TREE/$dir/candidates.sh" ] || continue
  home=(); while IFS= read -r g; do home+=("$g"); done < <(jq -r '.home[]' <<<"$m")
  # 1. the module's own search
  (cd "$SR_TREE" && "./$dir/candidates.sh") >"$work/cand" 2>"$work/err" ||
    refuse "$dir/candidates.sh failed, so leaks of that module cannot be found: $(head -c 300 "$work/err")"
  # 2. scope to the range, then drop the expected places
  whole=0; printf '%s\n' "$changed" | grep -Fxq -e "$dir/module.yaml" -e "$dir/candidates.sh" && whole=1
  adrs="$(jq -c --arg d "$dir" '[.[] | select(.frontmatter.modules // [] | index($d))]' <<<"$ADRS")"
  exc="$(jq -r '[.[] | .frontmatter.exceptions // [] | .[]] | .[]' <<<"$adrs")"
  left="[]"
  while IFS= read -r c; do
    [[ "$c" =~ ^(.+):([0-9]+):(.*)$ ]] || refuse "$dir/candidates.sh printed '$c', not path:line:snippet"
    p="${BASH_REMATCH[1]}"; l="${BASH_REMATCH[2]}"; t="${BASH_REMATCH[3]}"
    [ "$whole" = 1 ] || printf '%s\n' "$added" | grep -Fxq -- "$p:$l" || continue
    case "$p" in *_test.go) continue ;; esac
    in_globs "$p" "${home[@]}" && continue
    printf '%s\n' "$exc" | grep -Fxq -- "$p" && continue
    left="$(jq -c --arg p "$p" --argjson l "$l" --arg t "$t" '. + [{path: $p, line: $l, text: $t}]' <<<"$left")"
  done < <(grep -v '^[[:space:]]*$' "$work/cand")
  [ "$(jq 'length' <<<"$left")" -gt 0 ] || continue
  groups="$(jq -c --arg d "$dir" --argjson m "$m" --argjson a "$adrs" --argjson left "$left" \
    '. + [{module: $d, home: $m.home, api: $m.api, adrs: [$a[] | {id, text}], matches: $left}]' <<<"$groups")"
done < <(jq -c '.[]' <<<"$MODULES")
[ "$(jq 'length' <<<"$groups")" -gt 0 ] || { jq -n '{skip: true}'; exit 0; }
jq -n -c --argjson g "$groups" '{additionalContext: {modules: $g}}'
