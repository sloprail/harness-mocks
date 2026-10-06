#!/usr/bin/env bash
# For every module (a <dir>/module.yaml with `home` and `api`): go list over the
# committed tree; an importer outside the home that imports a package inside it
# which is not listed in api is a violation. Globs: `**` and `*` cross dirs.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/modules.sh"
load_modules; modules="$MODULES"
nmod="$(jq 'length' <<<"$modules")" || refuse_error "could not count the modules, so module boundaries cannot be checked"
[ "$nmod" -gt 0 ] || exit 0
[ -f "$SR_TREE/go.mod" ] || exit 0
mod="$(cd "$SR_TREE" && go list -m 2>/dev/null)" || refuse_error "go list -m failed in the committed tree, so module boundaries cannot be checked"
out="$(cd "$SR_TREE" && go list -f '{{.ImportPath}}{{range .Imports}} {{.}}{{end}}' ./... 2>&1)" ||
  refuse_error "go list failed in the committed tree, so module boundaries cannot be checked: $out"

rel() { case "$1" in "$mod"/*) printf '%s' "${1#"$mod"/}" ;; "$mod") printf '.' ;; *) return 1 ;; esac; }

# every lookup is captured first: a jq that fails would otherwise be an empty list (no module, no
# home, no api), and the rule would pass having checked nothing
mlist="$(jq -c '.[]' <<<"$modules")" || refuse_error "could not list the modules, so module boundaries cannot be checked"
problems=""
while IFS= read -r m; do
  [ -n "$m" ] || continue
  id="$(jq -r '.dir' <<<"$m")" || refuse_error "could not read a module's directory, so module boundaries cannot be checked"
  hlist="$(jq -r '.home[]' <<<"$m")" || refuse_error "could not read the home of module $id, so module boundaries cannot be checked"
  alist="$(jq -r '.api[]' <<<"$m")" || refuse_error "could not read the api of module $id, so module boundaries cannot be checked"
  home=(); api=()
  while IFS= read -r g; do [ -n "$g" ] && home+=("$g"); done <<<"$hlist"
  while IFS= read -r g; do [ -n "$g" ] && api+=("$g"); done <<<"$alist"
  while read -r pkg imports; do
    from="$(rel "$pkg")" || continue
    in_globs "$from" "${home[@]}" && continue
    for imp in $imports; do
      to="$(rel "$imp")" || continue
      in_globs "$to" "${home[@]}" || continue
      grep -Fxq -- "$to" <<<"$alist" && continue   # (a here-string, not a pipe: grep -q exits early and would fail a printf under pipefail)
      problems="${problems}- $from imports $to, inside module $id but not its api ($(IFS=,; echo "${api[*]}"))"$'\n'
    done
  done <<<"$out"
done <<<"$mlist"
[ -z "$problems" ] && exit 0
refuse "Module boundaries (use a module only through its api):
${problems}"
