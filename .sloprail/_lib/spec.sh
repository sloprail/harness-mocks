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
# spec/KIND/*.yaml. Unparseable YAML is refused, never skipped.
load_spec() {
  local kind="$1" f id doc
  SPEC="[]"
  for f in "$SR_TREE/spec/$kind"/*.yaml; do
    [ -f "$f" ] || continue
    id="$(basename "$f" .yaml)"
    doc="$(yq -o=json '.' "$f" 2>/dev/null)" || refuse "spec/$kind/$id.yaml is not valid YAML"
    SPEC="$(jq -c --arg id "$id" --arg p "spec/$kind/$id.yaml" --argjson d "${doc:-null}" '. + [{id: $id, path: $p, doc: $d}]' <<<"$SPEC")"
  done
}

# harnesses — the harness mocks in the committed tree, one name per line.
harnesses() { (cd "$SR_TREE" && for d in *-mock; do [ -d "$d" ] && echo "${d%-mock}"; done); }

