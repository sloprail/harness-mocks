#!/usr/bin/env bash
# ADR lookup. Source it; do not run it. Needs `refuse` (from changeset.sh, or
# define your own before sourcing).
#
# ADRs live at adr/<kebab-name>/ADR.md. Each one's frontmatter links the
# sloprails that enforce it:
#
#   sloprails: [file-guard/file-size, gate/file-size]
#
# Links are many-to-many: an ADR may be enforced by several rules, and a rule
# may enforce several ADRs. A rule finds the ADRs it enforces by its own
# qualified name, <nature>/<folder>, so nothing inside a rule names its ADR.

# adr_root — where ADRs are read from: the committed tree when judging a
# commit, the workspace when a gate runs before a write.
adr_root() { printf '%s/adr' "${SR_TREE:-${SR_WORKSPACE:-.}}"; }

# rule_qname — this rule's qualified name, e.g. file-guard/file-size.
rule_qname() {
  local d="${SR_GUARDRAIL_DIR:?SR_GUARDRAIL_DIR is not set}"
  printf '%s/%s' "$(basename "$(dirname "$d")")" "$(basename "$d")"
}

# load_adrs [QNAME] — sets ADRS to a JSON array of {id, path, frontmatter,
# text}: every ADR, or only those whose `sloprails` lists QNAME. An ADR whose
# frontmatter cannot be parsed is refused, never skipped: a skipped ADR is a
# decision nobody enforces.
load_adrs() {
  local want="${1:-}" f id fm
  ADRS="[]"
  for f in "$(adr_root)"/*/ADR.md; do
    [ -f "$f" ] || continue
    id="$(basename "$(dirname "$f")")"
    fm="$(yq --front-matter=extract -o=json '.' "$f" 2>/dev/null)" || refuse "adr/$id/ADR.md has frontmatter that is not valid YAML"
    [ -n "$fm" ] && [ "$fm" != "null" ] || fm="{}"
    if [ -n "$want" ] && ! jq -e --arg q "$want" '(.sloprails // []) | index($q)' <<<"$fm" >/dev/null; then
      continue
    fi
    ADRS="$(jq -c --arg id "$id" --arg p "adr/$id/ADR.md" --argjson fm "$fm" --rawfile t "$f" \
      '. + [{id: $id, path: $p, frontmatter: $fm, text: $t}]' <<<"$ADRS")"
  done
}
