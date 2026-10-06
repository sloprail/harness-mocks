#!/usr/bin/env bash
# The spec catalogs, read from the committed tree. Source after changeset.sh.
#
#   spec/invariants/<id>.yaml     statement                      (features: the user's words)
#   spec/capabilities/<id>.yaml   statement · providers.<harness>: {docs: [...], runs: [...]} | false
#
# The file name is the id. Harnesses are the top-level <harness>-mock/ dirs.
#
# Markers (one token after the kind, per the engine's marker grammar):
#   // sr:invariant <id>              code that upholds an invariant
#   // sr:proves <id>                 a test proving an invariant
#   // sr:capability <id>             a capability's one implementation, in internal/
#   // sr:provides <id>/<harness>     that harness's adapter for it
#   // sr:proves <id>/<harness>       a test proving it for that harness

# load_spec KIND — sets SPEC to a JSON array of {id, path, doc} for every
# spec/KIND/*.yaml. Unparseable YAML is refused, never skipped. A spec dir that is missing or holds
# no *.yaml is the tree (or the listing) failing, not a change with no specs: refuse_error, never
# an empty SPEC, which every coverage rule would read as "nothing to check" or as each id missing.
load_spec() {
  local kind="$1" out listed
  SPEC="[]"
  command -v yq >/dev/null 2>&1 && command -v jq >/dev/null 2>&1 ||
    refuse_error "yq and jq are needed to read spec/$kind and one is not on PATH, so the specs could not be read"
  [ -d "$SR_TREE/spec/$kind" ] ||
    refuse_error "spec/$kind is not in the committed tree at $SR_TREE (an incomplete tree?), so the specs could not be read"
  listed="$(ls "$SR_TREE/spec/$kind" 2>&1)" ||
    refuse_error "could not list spec/$kind: $listed"
  [[ "$listed" == *.yaml* ]] ||
    refuse_error "spec/$kind lists no *.yaml in the committed tree, so the specs could not be read (a failed or partial checkout?)"
  # one yq over every file: it names each document by its file
  out="$(yq -o=json -I=0 '{"id": (filename | split("/") | .[-1] | sub("\\.yaml$"; "")), "path": ("spec/'"$kind"'/" + (filename | split("/") | .[-1])), "doc": .}' \
    "$SR_TREE/spec/$kind"/*.yaml 2>&1)" || refuse "a file under spec/$kind is not valid YAML: $out"
  SPEC="$(jq -sc . <<<"$out")" || refuse_error "jq could not collect the specs under spec/$kind"
}

# harnesses — the harness mocks in the committed tree, one name per line. Prints nothing when it
# finds none or cannot look: callers refuse_error on an empty list (no cell is checked against it).
harnesses() { (cd "$SR_TREE" 2>/dev/null && for d in *-mock; do [ -d "$d" ] && echo "${d%-mock}"; done); }
