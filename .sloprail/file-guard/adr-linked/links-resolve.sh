#!/usr/bin/env bash
# Over every adr/<name>/ADR.md in the committed tree:
#   - the folder name is kebab-case starting with a letter (no numbers or
#     dates: rules find ADRs by link, git keeps the order), and holds ADR.md
#   - its frontmatter's shape (concern, sloprails, no status, …) is
#     file-guard/shapes' (schemas/adr.cue)
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
# a failed jq is a refusal, not an empty list of ADRs (which would pass)
list="$(jq -c '.[]' <<<"$ADRS")" || refuse_error "the ADRs could not be listed, so none could be checked"
while IFS= read -r a; do
  [ -n "$a" ] || continue
  id="$(jq -r '.id' <<<"$a")" || refuse_error "an ADR's id could not be read, so it could not be checked"
  fm="$(jq -c '.frontmatter' <<<"$a")" || refuse_error "adr/$id: its frontmatter could not be read, so it could not be checked"
  text="$(jq -r '.text' <<<"$a")" || refuse_error "adr/$id: its text could not be read, so it could not be checked"
  links="$(jq -r 'if (.sloprails | type) == "array" then .sloprails[] else empty end' <<<"$fm")" ||
    refuse_error "adr/$id: its linked rules could not be read, so it could not be checked"
  [ -n "$links" ] || add "adr/$id links no sloprail: list the rules that enforce it under 'sloprails:' (an ADR nothing enforces is prose)"
  while IFS= read -r l; do
    [ -n "$l" ] || continue
    nature="${l%%/*}"; rule="${l#*/}"
    case "$nature" in file-guard | gate | context) ;; *) add "adr/$id: '$l' must be <file-guard|gate|context>/<name>"; continue ;; esac
    [ -f "$SR_TREE/.sloprail/$nature/$rule/$nature.yaml" ] || add "adr/$id links '$l', but .sloprail/$nature/$rule/$nature.yaml does not exist"
  done <<<"$links"
  for sec in Concern Decision; do
    grep -Eq "^## $sec[[:space:]]*$" <<<"$text" || add "adr/$id has no '## $sec' section"
  done
done <<<"$list"

[ -z "$problems" ] && exit 0
refuse "ADRs that are not linked to their enforcement, or not in the ADR format:
${problems}"
