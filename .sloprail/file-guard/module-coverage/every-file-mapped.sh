#!/usr/bin/env bash
# `space` (globs) comes from the ADR(s) linking this rule. The ADR's
# `exceptions` list is empty and the rule honours none: every non-test Go file in
# the space must lie in exactly one module's home.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/modules.sh"
load_adrs "$(rule_qname)"
space=(); while IFS= read -r g; do [ -n "$g" ] && space+=("$g"); done < <(jq -r '[.[] | .frontmatter.space // [] | .[]] | .[]' <<<"$ADRS")
[ "${#space[@]}" -gt 0 ] || refuse "no ADR linking $(rule_qname) declares a space, so module coverage cannot be checked"
load_modules
files="$(git -C "$SR_TREE" ls-files -- '*.go' ':!*_test.go' 2>&1)" || refuse "could not list Go files: $files"

problems=""
while IFS= read -r f; do
  [ -n "$f" ] || continue
  in_globs "$f" "${space[@]}" || continue
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
[ -z "$problems" ] && exit 0
refuse "Code outside the module map (adr/modules-cover-code):
${problems}"
