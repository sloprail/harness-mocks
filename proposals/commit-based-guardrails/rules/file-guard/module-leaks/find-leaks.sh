#!/usr/bin/env bash
# prepare: steps 1 and 2 of module-leaks. Emits {"skip": true} when nothing is
# left to judge, else the leftover matches grouped by module, with the text of
# the ADRs linking each module (frontmatter `modules:`).
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/modules.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
load_modules; load_adrs
added="$(added_lines)"
changed="$(cs '.changeset.files[].path')"
work="$(mktemp -d "${TMPDIR:-/tmp}/module-leaks.XXXXXX")"; trap 'rm -rf "$work"' EXIT

groups="[]"
while IFS= read -r m; do
  [ -n "$m" ] || continue
  dir="$(jq -r '.dir' <<<"$m")"
  home=(); while IFS= read -r g; do home+=("$g"); done < <(jq -r '.home[]' <<<"$m")
  [ -x "$SR_TREE/$dir/signatures.sh" ] || continue
  (cd "$SR_TREE" && "./$dir/signatures.sh") >"$work/sig" 2>"$work/err" ||
    refuse "$dir/signatures.sh failed, so leaks of that module cannot be found: $(head -c 300 "$work/err")"
  grep -v '^[[:space:]]*$' "$work/sig" >"$work/pat" || continue
  adrs="$(jq -c --arg d "$dir" '[.[] | select(.frontmatter.modules // [] | index($d))]' <<<"$ADRS")"
  exc="$(jq -r '[.[] | .frontmatter.exceptions // [] | .[]] | .[]' <<<"$adrs")"
  # 1. candidate lines: the whole tree if the module's boundary changed, else added lines
  if printf '%s\n' "$changed" | grep -Fxq -e "$dir/module.yaml" -e "$dir/signatures.sh"; then
    (cd "$SR_TREE" && git grep -n -I -E -f "$work/pat" -- '*.go' ':!proposals/**') 2>/dev/null |
      awk -F: '{p = $1; l = $2; sub(/^[^:]*:[^:]*:/, ""); print p "\t" l "\t" $0}' >"$work/hits"
  else
    printf '%s\n' "$added" | awk -F'\t' 'NF >= 3' | while IFS=$'\t' read -r p l t; do
      printf '%s\n' "$t" | grep -Eq -f "$work/pat" && printf '%s\t%s\t%s\n' "$p" "$l" "$t"
    done >"$work/hits"
  fi
  # 2. drop the expected places
  left="[]"
  while IFS=$'\t' read -r p l t; do
    [ -n "$p" ] || continue
    case "$p" in *_test.go) continue ;; esac
    in_globs "$p" "${home[@]}" && continue
    printf '%s\n' "$exc" | grep -Fxq -- "$p" && continue
    left="$(jq -c --arg p "$p" --argjson l "$l" --arg t "$t" '. + [{path: $p, line: $l, text: $t}]' <<<"$left")"
  done <"$work/hits"
  [ "$(jq 'length' <<<"$left")" -gt 0 ] || continue
  groups="$(jq -c --arg d "$dir" --argjson m "$m" --argjson a "$adrs" --argjson left "$left" \
    '. + [{module: $d, home: $m.home, api: $m.api, adrs: [$a[] | {id, text}], matches: $left}]' <<<"$groups")"
done < <(jq -c '.[]' <<<"$MODULES")
[ "$(jq 'length' <<<"$groups")" -gt 0 ] || { jq -n '{skip: true}'; exit 0; }
jq -n -c --argjson g "$groups" '{additionalContext: {modules: $g}}'
