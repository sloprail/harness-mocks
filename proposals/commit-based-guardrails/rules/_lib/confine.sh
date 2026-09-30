#!/usr/bin/env bash
# confine REGEX ALLOWED_PATH_ERE LABEL — the shared deterministic half of an ADR
# that says "only <allowed> may do <regex>". Source after changeset.sh and
# adr.sh. Over the changeset's added and changed non-test Go files: a file
# whose path does not match ALLOWED_PATH_ERE may not contain REGEX, except a file listed under the
# linked ADRs' `exceptions`, which may not contain MORE matches than before.
# Prints nothing and returns when fine; refuses otherwise.
confine() {
  local re="$1" allowed="$2" label="$3" exc problems="" f path n o
  load_adrs "$(rule_qname)"
  exc="$(jq -r '[.[] | .frontmatter.exceptions // [] | .[]] | .[]' <<<"$ADRS")"
  count() { printf '%s\n' "$1" | grep -cE "$re" || true; }
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    path="$(jq -r '.path' <<<"$f")"
    case "$path" in *_test.go) continue ;; esac
    printf '%s' "$path" | grep -Eq -- "$allowed" && continue
    n="$(count "$(jq -r '.newContent // ""' <<<"$f")")"
    [ "$n" -gt 0 ] || continue
    if printf '%s\n' "$exc" | grep -Fxq -- "$path"; then
      o="$(count "$(jq -r '.oldContent // ""' <<<"$f")")"
      [ "$n" -le "$o" ] || problems="${problems}- $path adds a site ($o → $n); it is a legacy exception and may only lose them"$'\n'
    else
      problems="${problems}- $path $label"$'\n'
    fi
  done < <(cs_json '.changeset.files[] | select(.status != "D" and (.path | endswith(".go")))')
  [ -z "$problems" ] || refuse "$(rule_qname) (${ADR_TITLE:-see the linked ADR}):
${problems}"
}
