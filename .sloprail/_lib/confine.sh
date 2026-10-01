#!/usr/bin/env bash
# confine REGEX ALLOWED_PATH_ERE LABEL — the shared deterministic half of an ADR
# that says "only <allowed> may do <regex>". Source after changeset.sh and
# adr.sh.
#
# - A changed non-test Go file whose path does not match ALLOWED_PATH_ERE and is
#   not listed under the linked ADRs' `exceptions` may not contain REGEX.
# - The legacy files listed under `exceptions` may keep their sites, but the
#   TOTAL number of sites may not grow from the range's base to its head.
#   Counted over the whole tree outside ALLOWED_PATH_ERE, so a split that
#   carries a site into a new file changes nothing. (The new file must itself
#   be listed: the ADR's exception list is where legacy code is named.)
# Prints nothing and returns when fine; refuses otherwise.
confine() {
  local re="$1" allowed="$2" label="$3" exc problems="" f path base head n
  load_adrs "$(rule_qname)"
  exc="$(jq -r '[.[] | .frontmatter.exceptions // [] | .[]] | .[]' <<<"$ADRS")"
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    path="$(jq -r '.path' <<<"$f")"
    case "$path" in *_test.go) continue ;; esac
    printf '%s' "$path" | grep -Eq -- "$allowed" && continue
    printf '%s\n' "$exc" | grep -Fxq -- "$path" && continue
    n="$(jq -r '.newContent // ""' <<<"$f" | grep -cE "$re" || true)"
    [ "$n" -eq 0 ] || problems="${problems}- $path $label"$'\n'
  done < <(cs_json '.changeset.files[] | select(.status != "D" and (.path | endswith(".go")))')

  # The ratchet: total sites across the whole tree outside the allowed paths,
  # base vs head. A pure move keeps it equal whatever the files are called.
  base="$(cs '.changeset.base')"; head="$(cs '.changeset.head')"
  total() {   # REV
    # the rules' own scope: the shared internal/ and every harness mock
    git -C "$SR_TREE" grep -c -E "$re" "$1" -- ':(glob)internal/**/*.go' ':(glob)*-mock/**/*.go' ':!*_test.go' 2>/dev/null |
      sed "s#^$1:##" | grep -Ev "^(${allowed#^})" | awk -F: '{s += $NF} END {print s + 0}'
  }
  if [ -n "$base" ] && [ -n "$head" ]; then
    local tb th
    tb="$(total "$base")"; th="$(total "$head")"
    [ "$th" -le "$tb" ] ||
      problems="${problems}- the code outside the allowed place now holds $th sites, up from $tb: legacy sites may only be removed"$'\n'
  fi
  [ -z "$problems" ] || refuse "$(rule_qname) (${ADR_TITLE:-see the linked ADR}):
${problems}"
}
