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
  local re="$1" allowed="$2" label="$3" exc problems="" f path base head n files body rc
  load_adrs "$(rule_qname)"
  # every lookup below refuses when it fails: a failed one read as empty would find no site and pass
  exc="$(jq -r '[.[] | .frontmatter.exceptions // [] | .[]] | .[]' <<<"$ADRS")" ||
    refuse "the linked ADRs' exceptions could not be read, so no site could be judged"
  files="$(cs_json '.changeset.files[] | select(.status != "D" and (.path | endswith(".go")))')" ||
    refuse "the changed Go files could not be listed, so no site could be judged"
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    path="$(jq -r '.path' <<<"$f")" || refuse "a changed file's path could not be read, so no site could be judged"
    case "$path" in *_test.go) continue ;; esac
    # here-strings, not `printf | grep -q` (grep quitting early can SIGPIPE the printf); grep exits 1 on no match
    # (fine) and >1 on an error (refuse)
    grep -Eq -- "$allowed" <<<"$path"; rc=$?
    [ "$rc" -le 1 ] || refuse "$path: the allowed paths could not be matched, so its sites could not be counted"
    [ "$rc" -eq 0 ] && continue
    case $'\n'"$exc"$'\n' in *$'\n'"$path"$'\n'*) continue ;; esac
    body="$(jq -r '.newContent // ""' <<<"$f")" || refuse "$path: its content could not be read, so its sites could not be counted"
    n="$(grep -cE "$re" <<<"$body" || true)"
    [ "$n" -eq 0 ] || problems="${problems}- $path $label"$'\n'
  done <<<"$files"

  # The ratchet: total sites across the whole tree outside the allowed paths,
  # base vs head. A pure move keeps it equal whatever the files are called.
  base="$(cs '.changeset.base')" || refuse "the range's base could not be read, so the site count could not be compared"
  head="$(cs '.changeset.head')" || refuse "the range's head could not be read, so the site count could not be compared"
  total() {   # REV — sets TOTAL (no refuse here: this may run in a subshell)
    local out rc
    # the rules' own scope: the shared internal/ and every harness mock. git grep exits 1 on no match
    # (fine: zero sites) and >1 on an error (refuse)
    out="$(git -C "$SR_TREE" grep -c -E "$re" "$1" -- ':(glob)internal/**/*.go' ':(glob)*-mock/**/*.go' ':!*_test.go' 2>&1)"
    rc=$?
    [ "$rc" -le 1 ] || refuse "could not count the sites at $1: $out"
    TOTAL="$(printf '%s\n' "$out" | sed "s#^$1:##" | awk -F: -v re="$allowed" 'NF && $1 !~ re {s += $NF} END {print s + 0}')" ||
      refuse "could not total the sites at $1"
  }
  if [ -n "$base" ] && [ -n "$head" ]; then
    local tb th
    total "$base"; tb="$TOTAL"; total "$head"; th="$TOTAL"
    [ "$th" -le "$tb" ] ||
      problems="${problems}- the code outside the allowed place now holds $th sites, up from $tb: legacy sites may only be removed"$'\n'
  fi
  [ -z "$problems" ] || refuse "$(rule_qname) (${ADR_TITLE:-see the linked ADR}):
${problems}"
}
