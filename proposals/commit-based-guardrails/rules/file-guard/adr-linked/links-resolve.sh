#!/usr/bin/env bash
# Over every adr/<name>/ADR.md in the committed tree:
#   - the folder name is kebab-case starting with a letter (no numbers or
#     dates: rules find ADRs by link, git keeps the order), and holds ADR.md
#   - frontmatter: `concern` is a one-line string (the index concern-placement
#     judges against); `sloprails` is a non-empty list; `home`, `api` and
#     `exceptions` are lists when present
#   - there is no `status`: an ADR in the tree is in force; a retired one is
#     deleted (git keeps it)
#   - every sloprails entry is <nature>/<name> and names a rule folder that
#     exists in .sloprail/ (file-guard.yaml, gate.yaml or context.yaml)
#   - the body has the sections ## Concern and ## Decision
# Whether the linked rules enforce what the ADR decides is adr-matches-sloprails'
# judge; whether the prose is good is adr-well-formed's.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"

problems=""
add() { problems="${problems}- $1"$'\n'; }

for d in "$SR_TREE"/adr/*/; do
  [ -d "$d" ] || continue
  name="$(basename "$d")"
  printf '%s' "$name" | grep -Eq '^[a-z][a-z0-9]*(-[a-z0-9]+)*$' || add "adr/$name: the folder name must be kebab-case starting with a letter (no numbers or dates)"
  [ -f "$d/ADR.md" ] || add "adr/$name has no ADR.md"
done
load_adrs
while IFS= read -r a; do
  [ -n "$a" ] || continue
  id="$(jq -r '.id' <<<"$a")"
  fm="$(jq -c '.frontmatter' <<<"$a")"
  jq -e 'has("status") | not' <<<"$fm" >/dev/null ||
    add "adr/$id has a status: an ADR in the tree is in force; delete a retired one instead (git keeps it)"
  jq -e '(.concern | type) == "string" and (.concern | length) > 0 and (.concern | test("\n") | not)' <<<"$fm" >/dev/null ||
    add "adr/$id needs 'concern:', one line naming what it decides"
  for k in home api exceptions; do
    jq -e --arg k "$k" '(.[$k] == null) or (.[$k] | type == "array")' <<<"$fm" >/dev/null || add "adr/$id: '$k' must be a list"
  done
  links="$(jq -r 'if (.sloprails | type) == "array" then .sloprails[] else empty end' <<<"$fm")"
  [ -n "$links" ] || add "adr/$id links no sloprail: list the rules that enforce it under 'sloprails:' (an ADR nothing enforces is prose)"
  while IFS= read -r l; do
    [ -n "$l" ] || continue
    nature="${l%%/*}"; rule="${l#*/}"
    case "$nature" in file-guard | gate | context) ;; *) add "adr/$id: '$l' must be <file-guard|gate|context>/<name>"; continue ;; esac
    [ -f "$SR_TREE/.sloprail/$nature/$rule/$nature.yaml" ] || add "adr/$id links '$l', but .sloprail/$nature/$rule/$nature.yaml does not exist"
  done <<<"$links"
  for sec in Concern Decision; do
    jq -r '.text' <<<"$a" | grep -Eq "^## $sec[[:space:]]*$" || add "adr/$id has no '## $sec' section"
  done
done < <(jq -c '.[]' <<<"$ADRS")

[ -z "$problems" ] && exit 0
refuse "ADRs that are not linked to their enforcement, or not in the ADR format:
${problems}"
