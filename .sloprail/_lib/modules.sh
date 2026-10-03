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
  local files out
  MODULES="[]"
  files="$(git -C "$SR_TREE" ls-files -- '*module.yaml' ':!proposals/**' 2>&1)" || refuse "could not list module.yaml files: $files"
  [ -n "$files" ] || return 0
  # one yq over every module.yaml: it names each document by its file
  out="$(cd "$SR_TREE" && printf '%s\n' "$files" | xargs yq -o=json -I=0 '{"dir": (filename | sub("/?module\\.yaml$"; "") | (select(. != "") // ".")), "concern": (.concern // ""), "home": (.home // []), "api": (.api // [])}' 2>&1)" ||
    refuse "a module.yaml is not valid YAML: $out"
  MODULES="$(jq -sc . <<<"$out")"
}

# in_globs PATH GLOB… — PATH matches one of the globs (** and * both cross /).
in_globs() { local p="$1" g; shift; for g in "$@"; do g="${g//\*\*/*}"; [[ "$p" == $g || "$p/" == $g ]] && return 0; done; return 1; }

# module_home_files MODULE_JSON — "glob<TAB>path<TAB>object id" for every tracked file (outside
# proposals/) that each of the module's `home` globs matches, in one pass over `git ls-files -s`
# (in_globs' rules: * and ** both cross /, and a glob also matches the directory itself).
module_home_files() {
  local globs
  globs="$(jq -r '.home[]' <<<"$1")"
  [ -n "$globs" ] || return 0
  git -C "$SR_TREE" ls-files -s -- ':!proposals/**' | GLOBS="$globs" awk -F'\t' '
    function re(g,   i, c, out) {
      out = ""
      for (i = 1; i <= length(g); i++) {
        c = substr(g, i, 1)
        if (c == "*") { while (substr(g, i + 1, 1) == "*") i++; out = out ".*" }
        else if (c == "?") out = out "."
        else if (index(".+(){}|^$\\[]", c)) out = out "\\" c
        else out = out c
      }
      return "^" out "$"
    }
    BEGIN { n = split(ENVIRON["GLOBS"], g, "\n"); for (i = 1; i <= n; i++) r[i] = re(g[i]) }
    { split($1, m, " "); p = $2
      for (i = 1; i <= n; i++) if (p ~ r[i] || (p "/") ~ r[i]) print g[i] "\t" p "\t" m[2] }'
}
