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
  local want="${1:-}" files out texts
  ADRS="[]"
  files=("$(adr_root)"/*/ADR.md)
  [ -f "${files[0]}" ] || return 0
  # one awk lifts every frontmatter (stamped with its file), one yq reads them all, one jq the texts
  out="$(awk 'FNR == 1 { inside = ($0 == "---"); print "---"; print "__path: \"" FILENAME "\""; next }
              inside && $0 == "---" { inside = 0; next }
              inside { print }' "${files[@]}" | yq -o=json -I=0 '{"path": .__path, "frontmatter": (del(.__path) | . // {})}' 2>&1)" ||
    refuse "an ADR.md has frontmatter that is not valid YAML: $out"
  # a failed lookup refuses (an empty ADR list would be "no decision to enforce"); the texts go
  # to jq through a file, not argv (Linux caps one argument at 128 KB)
  texts="$(jq -Rn 'reduce inputs as $l ({}; .[input_filename] += $l + "\n")' "${files[@]}")" ||
    refuse_error "the ADR texts could not be read, so no ADR could be checked"
  ADRS="$(jq -sc --slurpfile t0 <(printf '%s' "$texts") --arg want "$want" --arg root "$(adr_root)/" '
    [.[] | .path as $f | (($f | ltrimstr($root)) | split("/")[0]) as $id
     | select($want == "" or ((.frontmatter | type) == "object" and ((.frontmatter.sloprails // []) | index($want))))
     | {id: $id, path: ("adr/" + $id + "/ADR.md"), frontmatter: (if (.frontmatter | type) == "object" then .frontmatter else {} end), text: ($t0[0][$f] // "")}]' <<<"$out")" ||
    refuse_error "the ADRs could not be listed, so no ADR could be checked"
}
