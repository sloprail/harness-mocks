#!/usr/bin/env bash
# Modules: a <dir>/module.yaml declares a boundary (home, api). Source after changeset.sh.
#
# A module may also ship <dir>/candidates.sh, which owns the whole search for
# its logic: run from the root of the tree being judged, it prints every line
# that looks like the module's work, one `path:line:snippet` per line (the
# format of `git grep -n`). The rules decide what is expected; the module only
# says where its logic appears.

# load_modules — sets MODULES to a JSON array of {dir, home, api} for every
# module.yaml in the committed tree. Unparseable is refused, never skipped.
load_modules() {
  local f m out
  MODULES="[]"
  out="$(git -C "$SR_TREE" ls-files -- '*module.yaml' ':!proposals/**' 2>&1)" || refuse "could not list module.yaml files: $out"
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    m="$(yq -o=json '.' "$SR_TREE/$f" 2>/dev/null)" || refuse "$f is not valid YAML"
    MODULES="$(jq -c --arg d "$(dirname "$f")" --argjson m "$m" '. + [{dir: $d, home: ($m.home // []), api: ($m.api // [])}]' <<<"$MODULES")"
  done <<<"$out"
}

# in_globs PATH GLOB… — PATH matches one of the globs (** and * both cross /).
in_globs() { local p="$1" g; shift; for g in "$@"; do g="${g//\*\*/*}"; [[ "$p" == $g || "$p/" == $g ]] && return 0; done; return 1; }
