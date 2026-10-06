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

# Fails (non-zero) when what the changeset touches cannot be worked out (the markers cannot be read, or
# a diff or a file's content cannot be had): "nothing touched" is an empty table, a failure is not, so
# a caller runs `load_touched_markers || refuse ...` and never reads a failed lookup as "no capability
# touched" (which would skip the judge).
load_touched_markers() {
  [ -z "${TM_READY:-}" ] || return 0
  TOUCHED_MARKERS_TSV=""
  local base head all rows path st side kind fqn line files f nl ol lines chg hit diff content
  base="$(cs '.changeset.base')" || return 1; head="$(cs '.changeset.head')" || return 1
  all="$(printf '%s' "$payload" | jq -r '.changeset.files[] | . as $f
    | ((.newMarkers // []) | map(["n", $f.path, $f.status, .kind, .fqn, (.line | tostring)] | @tsv))[],
      ((.oldMarkers // []) | map(["o", $f.path, $f.status, .kind, .fqn, (.line | tostring)] | @tsv))[]')" || return 1
  rows="$(printf '%s\n' "$all" | awk -F'\t' '$4 == "capability" || $4 == "provides" || $4 == "proves"')" || return 1
  [ -n "$rows" ] || { TM_READY=1; return 0; }
  # added, deleted, renamed or unreadable: every marker of the file; modified: those whose region changed
  TOUCHED_MARKERS_TSV="$(printf '%s\n' "$rows" | awk -F'\t' '$3 != "M" {print $2 "\t" $4 "\t" $5}')" || return 1
  files="$(printf '%s\n' "$rows" | awk -F'\t' '$3 == "M" {print $2}' | sort -u)" || return 1
  for f in $files; do
    if [ -z "$base" ] || [ -z "$head" ] || ! git -C "$SR_TREE" cat-file -e "$head:$f" 2>/dev/null; then
      TOUCHED_MARKERS_TSV="${TOUCHED_MARKERS_TSV}"$'\n'"$(printf '%s\n' "$rows" | awk -F'\t' -v f="$f" '$2 == f {print $2 "\t" $4 "\t" $5}')"; continue
    fi
    # a diff that cannot be had is not "no changed lines"
    diff="$(git -C "$SR_TREE" diff -U0 --no-color --no-renames "$base" "$head" -- "$f" 2>/dev/null)" || return 1
    lines="$(printf '%s\n' "$diff" | diff_lines)" || return 1
    for side in n o; do
      chg="$(printf '%s\n' "$lines" | awk -v s="$side" '$1 == s {print $2}' | tr '\n' ' ')"
      [ -n "$chg" ] || continue
      nl="$(printf '%s\n' "$rows" | awk -F'\t' -v f="$f" -v s="$side" '$2 == f && $1 == s {print $6}' | tr '\n' ' ')"
      [ -n "$nl" ] || continue
      if [ "$side" = n ]; then content="$(git -C "$SR_TREE" show "$head:$f")" || return 1
      else content="$(git -C "$SR_TREE" show "$base:$f")" || return 1; fi
      hit="$(printf '%s\n' "$content" | marker_regions "$nl" "$chg")" || return 1
      for line in $hit; do
        TOUCHED_MARKERS_TSV="${TOUCHED_MARKERS_TSV}"$'\n'"$(printf '%s\n' "$rows" | awk -F'\t' -v f="$f" -v s="$side" -v l="$line" '$2 == f && $1 == s && $6 == l {print $2 "\t" $4 "\t" $5}')"
      done
    done
  done
  TOUCHED_MARKERS_TSV="$(printf '%s\n' "$TOUCHED_MARKERS_TSV" | sed '/^$/d' | sort -u)" || return 1
  TM_READY=1
}
