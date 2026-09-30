#!/usr/bin/env bash
# For every module (a <dir>/module.yaml with `home` and `api`): go list over the
# committed tree; an importer outside the home that imports a package inside it
# which is not listed in api is a violation. Globs: `**` and `*` cross dirs.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/modules.sh"
load_modules; modules="$MODULES"
[ "$(jq 'length' <<<"$modules")" -gt 0 ] || exit 0
[ -f "$SR_TREE/go.mod" ] || exit 0
mod="$(cd "$SR_TREE" && go list -m 2>/dev/null)" || refuse "go list -m failed in the committed tree, so module boundaries cannot be checked"
out="$(cd "$SR_TREE" && go list -f '{{.ImportPath}}{{range .Imports}} {{.}}{{end}}' ./... 2>&1)" ||
  refuse "go list failed in the committed tree, so module boundaries cannot be checked: $out"

rel() { case "$1" in "$mod"/*) printf '%s' "${1#"$mod"/}" ;; "$mod") printf '.' ;; *) return 1 ;; esac; }

problems=""
while IFS= read -r m; do
  id="$(jq -r '.dir' <<<"$m")"
  home=(); api=()
  while IFS= read -r g; do home+=("$g"); done < <(jq -r '.home[]' <<<"$m")
  while IFS= read -r g; do api+=("$g"); done < <(jq -r '.api[]' <<<"$m")
  while read -r pkg imports; do
    from="$(rel "$pkg")" || continue
    in_globs "$from" "${home[@]}" && continue
    for imp in $imports; do
      to="$(rel "$imp")" || continue
      in_globs "$to" "${home[@]}" || continue
      printf '%s\n' "${api[@]}" | grep -Fxq -- "$to" && continue
      problems="${problems}- $from imports $to, inside module $id but not its api ($(IFS=,; echo "${api[*]}"))"$'\n'
    done
  done <<<"$out"
done < <(jq -c '.[]' <<<"$modules")
[ -z "$problems" ] && exit 0
refuse "Module boundaries (use a module only through its api):
${problems}"
