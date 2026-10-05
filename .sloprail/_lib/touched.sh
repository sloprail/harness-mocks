#!/usr/bin/env bash
# What a changeset really touches, narrower than "the file changed". Source after changeset.sh
# and snapshots.sh.
#
# A capability's judges read the declarations its markers sit on, its recordings and its cell. An
# edit to one function in a file that also carries other capabilities' markers is no reason to
# judge theirs. A doc page is NOT in this list: recordings own the truth and a doc is re-frozen
# only together with one (capture.sh), so a MANIFEST doc entry changing touches no capability and
# is in no verdict's key (adr/pinned-harness-versions). So:
#
#   load_touched_markers TOUCHED_MARKERS_TSV  "<path>\t<kind>\t<fqn>" per capability marker
#                                          whose declaration (the comment block it sits in and
#                                          the declaration that follows, to its closing brace)
#                                          has a changed line, or whose file was added, deleted
#                                          or renamed

# diff_lines — stdin: a unified diff; one "o N" per removed line and "n N" per added line.
diff_lines() {
  awk '/^@@ /{ match($0, /-[0-9]+/); o = substr($0, RSTART + 1, RLENGTH - 1) + 0; match($0, /\+[0-9]+/); n = substr($0, RSTART + 1, RLENGTH - 1) + 0; inh = 1; next }
       !inh { next }
       /^\+/ { print "n " n; n++; next }
       /^-/  { print "o " o; o++; next }
       /^ /  { o++; n++; next }'
}

# marker_regions MARKLINES CHANGED — stdin: a file; MARKLINES: the marker lines, space-separated;
# CHANGED: the changed line numbers of this side, space-separated. Prints the marker lines whose
# region (comment block + the declaration under it, to its closing "}" or ")" at the same
# indent, or to the end of a one-line declaration) holds a changed line.
marker_regions() {
  awk -v marks="$1" -v chg="$2" '
    function cm(s) { return s ~ /^[ \t]*(\/\/|#|--)/ }
    function ind(s) { match(s, /^[ \t]*/); return RLENGTH }
    { line[NR] = $0 }
    END {
      nm = split(marks, M, " "); nc = split(chg, C, " ")
      for (i = 1; i <= nm; i++) {
        m = M[i] + 0; s = m; while (s > 1 && cm(line[s - 1])) s--
        d = m + 1; while (d <= NR && cm(line[d])) d++
        e = m
        if (d <= NR) {
          e = d
          if (line[d] ~ /[{(][ \t]*$/) {
            id = ind(line[d]); e = NR
            for (j = d + 1; j <= NR; j++) if (line[j] !~ /^[ \t]*$/ && ind(line[j]) <= id && line[j] ~ /^[ \t]*[})]/) { e = j; break }
          }
        }
        for (k = 1; k <= nc; k++) if (C[k] + 0 >= s && C[k] + 0 <= e) { print m; break }
      }
    }'
}

load_touched_markers() {
  [ -z "${TM_READY:-}" ] || return 0
  TM_READY=1; TOUCHED_MARKERS_TSV=""
  local base head rows path st side kind fqn line files f nl ol lines chg hit
  base="$(cs '.changeset.base')"; head="$(cs '.changeset.head')"
  rows="$(printf '%s' "$payload" | jq -r '.changeset.files[] | . as $f
    | ((.newMarkers // []) | map(["n", $f.path, $f.status, .kind, .fqn, (.line | tostring)] | @tsv))[],
      ((.oldMarkers // []) | map(["o", $f.path, $f.status, .kind, .fqn, (.line | tostring)] | @tsv))[]' |
    awk -F'\t' '$4 == "capability" || $4 == "provides" || $4 == "proves"')"
  [ -n "$rows" ] || return 0
  # added, deleted, renamed or unreadable: every marker of the file; modified: those whose region changed
  TOUCHED_MARKERS_TSV="$(printf '%s\n' "$rows" | awk -F'\t' '$3 != "M" {print $2 "\t" $4 "\t" $5}')"
  files="$(printf '%s\n' "$rows" | awk -F'\t' '$3 == "M" {print $2}' | sort -u)"
  for f in $files; do
    lines="$(git -C "$SR_TREE" diff -U0 --no-color --no-renames "$base" "$head" -- "$f" 2>/dev/null | diff_lines)" || lines=""
    if [ -z "$base" ] || [ -z "$head" ] || ! git -C "$SR_TREE" cat-file -e "$head:$f" 2>/dev/null; then
      TOUCHED_MARKERS_TSV="${TOUCHED_MARKERS_TSV}"$'\n'"$(printf '%s\n' "$rows" | awk -F'\t' -v f="$f" '$2 == f {print $2 "\t" $4 "\t" $5}')"; continue
    fi
    for side in n o; do
      chg="$(printf '%s\n' "$lines" | awk -v s="$side" '$1 == s {print $2}' | tr '\n' ' ')"
      [ -n "$chg" ] || continue
      nl="$(printf '%s\n' "$rows" | awk -F'\t' -v f="$f" -v s="$side" '$2 == f && $1 == s {print $6}' | tr '\n' ' ')"
      [ -n "$nl" ] || continue
      if [ "$side" = n ]; then hit="$(git -C "$SR_TREE" show "$head:$f" | marker_regions "$nl" "$chg")"
      else hit="$(git -C "$SR_TREE" show "$base:$f" | marker_regions "$nl" "$chg")"; fi
      for line in $hit; do
        TOUCHED_MARKERS_TSV="${TOUCHED_MARKERS_TSV}"$'\n'"$(printf '%s\n' "$rows" | awk -F'\t' -v f="$f" -v s="$side" -v l="$line" '$2 == f && $1 == s && $6 == l {print $2 "\t" $4 "\t" $5}')"
      done
    done
  done
  TOUCHED_MARKERS_TSV="$(printf '%s\n' "$TOUCHED_MARKERS_TSV" | sed '/^$/d' | sort -u)"
}
