#!/usr/bin/env bash
# Runs tools/moveonly on each move-only commit against its own parent.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
[ -d "$SR_TREE/tools/moveonly" ] || refuse "move-only commits are in the range, but tools/moveonly does not exist to check them"
problems=""
while IFS= read -r c; do
  [ -n "$c" ] || continue
  sha="$(jq -r '.sha' <<<"$c")"
  renames=()
  for v in $(jq -r '.trailers["Sloprail-Refactor"][]? | select(startswith("move-only")) | ltrimstr("move-only") ' <<<"$c"); do
    renames+=(--rename "$v")
  done
  out="$(cd "$SR_TREE" && go run ./tools/moveonly --base "$sha^" --head "$sha" ${renames[@]+"${renames[@]}"} 2>&1)" ||
    problems="${problems}- $sha is marked move-only but is not:
$(printf '%s' "$out" | head -40)"$'\n'
done < <(cs_json '.changeset.commits[] | select(any(.trailers["Sloprail-Refactor"][]?; startswith("move-only")))')
[ -z "$problems" ] && exit 0
refuse "Commits marked Sloprail-Refactor: move-only that change more than where code lives:
${problems}Split the move and the change into separate commits; only the pure move carries the trailer."
