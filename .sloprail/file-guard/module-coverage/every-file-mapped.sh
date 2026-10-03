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
space=(); while IFS= read -r g; do [ -n "$g" ] && space+=("$g"); done < <(jq -r '[.[] | .frontmatter.space // [] | .[]] | .[]' <<<"$ADRS")
exc=(); while IFS= read -r g; do [ -n "$g" ] && exc+=("$g"); done < <(jq -r '[.[] | .frontmatter.exceptions // [] | .[]] | .[]' <<<"$ADRS")
[ "${#space[@]}" -gt 0 ] || refuse "no ADR linking $(rule_qname) declares a space, so module coverage cannot be checked"
load_modules
# every module states its concern (adr/modules-cover-code): an empty `concern:` cannot be judged
noconcern="$(jq -r '.[] | select((.concern | tostring | gsub("\\s"; "")) == "") | .dir' <<<"$MODULES")" || refuse "could not read the modules' concern lines"
[ -z "$noconcern" ] || refuse "these modules have no \`concern:\` line in their module.yaml; state each module's responsibility there (adr/modules-cover-code):
$noconcern"
files="$(git -C "$SR_TREE" ls-files -- '*.go' ':!*_test.go' ':!proposals/**' 2>&1)" || refuse "could not list Go files: $files"

problems=""
while IFS= read -r f; do
  [ -n "$f" ] || continue
  in_globs "$f" "${space[@]}" || continue
  [ "${#exc[@]}" -gt 0 ] && in_globs "$f" "${exc[@]}" && continue
  owners=""
  while IFS= read -r m; do
    home=(); while IFS= read -r g; do home+=("$g"); done < <(jq -r '.home[]' <<<"$m")
    [ "${#home[@]}" -gt 0 ] && in_globs "$f" "${home[@]}" && owners="${owners:+$owners, }$(jq -r '.dir' <<<"$m")"
  done < <(jq -c '.[]' <<<"$MODULES")
  case "$owners" in
    "") problems="${problems}- $f belongs to no module: put it in a module's home, or add a module.yaml for it"$'\n' ;;
    *,*) problems="${problems}- $f lies in more than one module's home ($owners): module homes may not overlap"$'\n' ;;
  esac
done <<<"$files"
# exceptions never grow
if [ "${#exc[@]}" -gt 0 ]; then
  base="$(cs '.changeset.base')"
  [ -n "$base" ] || refuse "the range's base is unknown, so it cannot be told whether the exceptions grew"
  bexc=()
  for id in $(jq -r '.[].id' <<<"$ADRS"); do
    txt="$(git -C "$SR_TREE" show "$base:adr/$id/ADR.md" 2>/dev/null)" || continue   # a new ADR: nothing excepted before it
    fm="$(printf '%s\n' "$txt" | awk 'NR == 1 && $0 == "---" { i = 1; next } i && $0 == "---" { exit } i { print }' | yq -o=json -I=0 '.' 2>&1)" ||
      refuse "adr/$id/ADR.md at the range's base has frontmatter that is not valid YAML, so its exceptions cannot be compared: $fm"
    jq -e --arg q "$(rule_qname)" '(.sloprails // []) | index($q)' <<<"${fm:-null}" >/dev/null 2>&1 || continue
    while IFS= read -r g; do [ -n "$g" ] && bexc+=("$g"); done < <(jq -r '.exceptions // [] | .[]' <<<"$fm")
  done
  tracked="$(git -C "$SR_TREE" ls-files -- ':!proposals/**' 2>&1)" || refuse "could not list the tracked files: $tracked"
  for g in "${exc[@]}"; do
    printf '%s\n' "${bexc[@]+"${bexc[@]}"}" | grep -Fxq -- "$g" && continue
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
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    in_globs "$f" "${exc[@]}" && problems="${problems}- $f is added under an exception: new code belongs to a module, never to the exceptions"$'\n'
  done < <(cs '.changeset.files[] | select((.status == "A" or .status == "R") and (.path | endswith(".go")) and (.path | endswith("_test.go") | not)) | .path')
fi
[ -z "$problems" ] && exit 0
refuse "Code outside the module map (adr/modules-cover-code):
${problems}"
