#!/usr/bin/env bash
# `space` and `exceptions` (globs) come from the ADR(s) linking this rule.
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
files="$(git -C "$SR_TREE" ls-files -- '*.go' ':!*_test.go' 2>&1)" || refuse "could not list Go files: $files"

problems=""
checked=0
while IFS= read -r f; do
  [ -n "$f" ] || continue
  in_globs "$f" "${space[@]}" || continue
  checked=$((checked + 1))
  owners=""
  while IFS= read -r m; do
    home=(); while IFS= read -r g; do home+=("$g"); done < <(jq -r '.home[]' <<<"$m")
    [ "${#home[@]}" -gt 0 ] && in_globs "$f" "${home[@]}" && owners="${owners:+$owners, }$(jq -r '.dir' <<<"$m")"
  done < <(jq -c '.[]' <<<"$MODULES")
  if [ "${#exc[@]}" -gt 0 ] && in_globs "$f" "${exc[@]}"; then
    [ -z "$owners" ] || problems="${problems}- $f matches \`exceptions\` yet lies in a module's home ($owners): it is mapped, so drop it from the exceptions"$'\n'
    [ -n "$(cs ".changeset.files[] | select(.path == $(jq -Rn --arg p "$f" '$p') and .status == \"A\") | .path")" ] &&
      problems="${problems}- $f is new under an \`exceptions\` glob: nothing new is added there; map it to a module"$'\n'
    continue
  fi
  case "$owners" in
    "") problems="${problems}- $f belongs to no module: put it in a module's home, or add a module.yaml for it"$'\n' ;;
    *,*) problems="${problems}- $f lies in more than one module's home ($owners): module homes may not overlap"$'\n' ;;
  esac
done <<<"$files"
[ "$checked" -gt 0 ] || refuse "no Go file in the tree matches the ADR's space, so module coverage was not checked: fix the space globs of adr/modules-cover-code"
[ -z "$problems" ] && exit 0
refuse "Code outside the module map (adr/modules-cover-code):
${problems}"
