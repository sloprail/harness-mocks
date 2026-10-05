#!/usr/bin/env bash
# `space` and `exceptions` (globs) come from the ADR(s) linking this rule.
# Exceptions only shrink (adr/modules-cover-code: "nothing new is added there"): the list may not
# grow against the range's base (an entry that covers more than the base's entries did), and a
# file ADDED in the range (or renamed in) may not match one.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/modules.sh"
load_adrs "$(rule_qname)"
# a failed load or lookup would read as "no exceptions" or "no space": refuse, never go on without them
jq -e 'type == "array"' <<<"$ADRS" >/dev/null 2>&1 || refuse_error "the ADRs linking $(rule_qname) could not be loaded, so module coverage cannot be checked"
spacelist="$(jq -r '[.[] | .frontmatter.space // [] | .[]] | .[]' <<<"$ADRS")" || refuse_error "could not read the space of the ADRs linking $(rule_qname), so module coverage cannot be checked"
exclist="$(jq -r '[.[] | .frontmatter.exceptions // [] | .[]] | .[]' <<<"$ADRS")" || refuse_error "could not read the exceptions of the ADRs linking $(rule_qname), so module coverage cannot be checked"
space=(); while IFS= read -r g; do [ -n "$g" ] && space+=("$g"); done <<<"$spacelist"
exc=(); while IFS= read -r g; do [ -n "$g" ] && exc+=("$g"); done <<<"$exclist"
[ "${#space[@]}" -gt 0 ] || refuse_error "no ADR linking $(rule_qname) declares a space, so module coverage cannot be checked"
load_modules
# every module states its concern (adr/modules-cover-code): an empty `concern:` cannot be judged
noconcern="$(jq -r '.[] | select((.concern | tostring | gsub("\\s"; "")) == "") | .dir' <<<"$MODULES")" || refuse_error "could not read the modules' concern lines"
[ -z "$noconcern" ] || refuse "these modules have no \`concern:\` line in their module.yaml; state each module's responsibility there (adr/modules-cover-code):
$noconcern"
files="$(git -C "$SR_TREE" ls-files -- '*.go' ':!*_test.go' ':!proposals/**' 2>&1)" || refuse_error "could not list Go files: $files"

mlist="$(jq -c '.[]' <<<"$MODULES")" || refuse_error "could not list the modules, so module coverage cannot be checked"
problems=""
while IFS= read -r f; do
  [ -n "$f" ] || continue
  in_globs "$f" "${space[@]}" || continue
  [ "${#exc[@]}" -gt 0 ] && in_globs "$f" "${exc[@]}" && continue
  owners=""
  while IFS= read -r m; do
    [ -n "$m" ] || continue
    hlist="$(jq -r '.home[]' <<<"$m")" || refuse_error "could not read a module's home, so module coverage cannot be checked"
    home=(); while IFS= read -r g; do [ -n "$g" ] && home+=("$g"); done <<<"$hlist"
    if [ "${#home[@]}" -gt 0 ] && in_globs "$f" "${home[@]}"; then
      mdir="$(jq -r '.dir' <<<"$m")" || refuse_error "could not read a module's directory, so module coverage cannot be checked"
      owners="${owners:+$owners, }$mdir"
    fi
  done <<<"$mlist"
  case "$owners" in
    "") problems="${problems}- $f belongs to no module: put it in a module's home, or add a module.yaml for it"$'\n' ;;
    *,*) problems="${problems}- $f lies in more than one module's home ($owners): module homes may not overlap"$'\n' ;;
  esac
done <<<"$files"
# exceptions never grow
if [ "${#exc[@]}" -gt 0 ]; then
  base="$(cs '.changeset.base')"
  [ -n "$base" ] || refuse_error "the range's base is unknown, so it cannot be told whether the exceptions grew"
  bexc=()
  ids="$(jq -r '.[].id' <<<"$ADRS")" || refuse_error "could not list the ADRs, so it cannot be told whether the exceptions grew"
  for id in $ids; do
    if ! git -C "$SR_TREE" cat-file -e "$base:adr/$id/ADR.md" 2>/dev/null; then
      # absent at the base: a new ADR, nothing excepted before it; an unreadable base is a tooling error, not "new"
      git -C "$SR_TREE" cat-file -e "$base^{commit}" 2>/dev/null ||
        refuse_error "the range's base $base could not be read, so adr/$id/ADR.md cannot be told from a new ADR"
      continue
    fi
    txt="$(git -C "$SR_TREE" show "$base:adr/$id/ADR.md" 2>/dev/null)" ||
      refuse_error "adr/$id/ADR.md at the range's base could not be read, so its exceptions cannot be compared"
    # (awk reads a here-string, not a pipe: it exits at the closing ---, and a printf still writing to it
    # would die of SIGPIPE and fail the whole pipeline under pipefail)
    fm="$(awk 'NR == 1 && $0 == "---" { i = 1; next } i && $0 == "---" { exit } i { print }' <<<"$txt" | yq -o=json -I=0 '.' 2>&1)" ||
      refuse_error "adr/$id/ADR.md at the range's base has frontmatter that is not valid YAML, so its exceptions cannot be compared: $fm"
    # exit 1 is "not linked to this rule" (skip it); any other failure is a lookup that did not work
    jq -e --arg q "$(rule_qname)" '(.sloprails // []) | index($q)' <<<"${fm:-null}" >/dev/null 2>&1; rc=$?
    [ "$rc" -le 1 ] || refuse_error "adr/$id/ADR.md at the range's base could not be read for its links, so its exceptions cannot be compared"
    [ "$rc" = 0 ] || continue
    bexclist="$(jq -r '.exceptions // [] | .[]' <<<"$fm")" || refuse_error "could not read the exceptions of adr/$id/ADR.md at the range's base, so they cannot be compared"
    while IFS= read -r g; do [ -n "$g" ] && bexc+=("$g"); done <<<"$bexclist"
  done
  tracked="$(git -C "$SR_TREE" ls-files -- ':!proposals/**' 2>&1)" || refuse_error "could not list the tracked files: $tracked"
  for g in "${exc[@]}"; do
    grep -Fxq -- "$g" <<<"$(printf '%s\n' "${bexc[@]+"${bexc[@]}"}")" && continue   # (a here-string: grep -q exits early, and a printf into a pipe would fail it under pipefail)
    # a new entry is a narrowing only when it matches something and everything it matches the base's entries matched
    hit=0; wider=""
    while IFS= read -r f; do
      [ -n "$f" ] || continue
      in_globs "$f" "$g" || continue
      hit=1
      [ "${#bexc[@]}" -gt 0 ] && in_globs "$f" "${bexc[@]}" || { wider="$f"; break; }
    done <<<"$tracked"
    if [ "$hit" = 0 ]; then problems="${problems}- exceptions grew: '$g' is new and matches no file, so it cannot be shown to narrow the existing entries"$'\n'
    elif [ -n "$wider" ]; then problems="${problems}- exceptions grew: '$g' is new and covers $wider, which no exception of the range's base covered"$'\n'; fi
  done
  # nothing new lands under an exception
  addedgo="$(cs '.changeset.files[] | select((.status == "A" or .status == "R") and (.path | endswith(".go")) and (.path | endswith("_test.go") | not)) | .path')" ||
    refuse_error "could not list the Go files the range adds, so it cannot be told whether new code landed under an exception"
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    in_globs "$f" "${exc[@]}" && problems="${problems}- $f is added under an exception: new code belongs to a module, never to the exceptions"$'\n'
  done <<<"$addedgo"
fi
[ -z "$problems" ] && exit 0
refuse "Code outside the module map (adr/modules-cover-code):
${problems}"
