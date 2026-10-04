#!/usr/bin/env bash
# What a changeset really touches, narrower than "the file changed". Source after changeset.sh
# and snapshots.sh.
#
# A capability's judges read (a) the doc pages its cells cite, each frozen by its own sha256 in a
# harness MANIFEST, and (b) the declarations its markers sit on. A re-frozen page is no reason to
# judge a capability that cites a different page, and an edit to one function in a file that
# also carries other capabilities' markers is no reason to judge theirs. So:
#
#   load_doc_changes     DOC_CHANGES_TSV   "<harness>\t<url>" per doc entry added, removed or
#                                          re-hashed (anchor dropped); "<harness>\t*" when the
#                                          MANIFEST was added, deleted or cannot be compared
#                                          (every page of that harness is then in question).
#                                          Nothing else in a MANIFEST counts: not its `pin` (which
#                                          harness binary capture.sh runs next; a recording's own
#                                          version is in its run.yaml) and not a page's `fetched`
#                                          date, so a binary bump or a re-freeze that finds the
#                                          same sha256 invalidates nothing
#   load_doc_shas        DOC_SHAS          JSON {<harness>: {docs: {<url>: <sha256>}}}
#                                          from the head tree: what a verdict's key must carry
#                                          for each page it reads, in place of the whole MANIFEST
#   load_touched_markers TOUCHED_MARKERS_TSV  "<path>\t<kind>\t<fqn>" per capability marker
#                                          whose declaration (the comment block it sits in and
#                                          the declaration that follows, to its closing brace)
#                                          has a changed line, or whose file was added, deleted
#                                          or renamed

load_doc_changes() {
  [ -z "${DOC_CHANGES_READY:-}" ] || return 0
  DOC_CHANGES_READY=1
  local out
  if out="$(printf '%s' "$payload" | jq -c '[.changeset.files[] | select(.path | test("^[a-z0-9]+-mock/snapshots/MANIFEST\\.yaml$"))
        | {h: (.path | split("-mock/")[0]), st: .status, old: ((.oldContent // "") | if . == "" then "null" else . end), new: ((.newContent // "") | if . == "" then "null" else . end)}]' |
      yq -p=json -o=json -I=0 '.[] | .old |= (@yamld) | .new |= (@yamld)' 2>/dev/null |
      jq -r '. as $f | if $f.st != "M" or ($f.old | type) != "object" or ($f.new | type) != "object"
               then [$f.h, "*"] | @tsv
               else (($f.old.docs // {}) as $o | ($f.new.docs // {}) as $n | ($o + $n) | keys[] as $u
                     | select(($o[$u].sha256 // "") != ($n[$u].sha256 // "")) | [$f.h, $u] | @tsv) end' 2>/dev/null)"; then
    DOC_CHANGES_TSV="$out"
  else   # cannot compare: every harness whose MANIFEST changed is wholly in question (the cautious side)
    DOC_CHANGES_TSV="$(cs '.changeset.files[].path | select(test("^[a-z0-9]+-mock/snapshots/MANIFEST\\.yaml$")) | sub("-mock/.*$"; "") + "\t*"')"
  fi
}

load_doc_shas() {
  [ -z "${DOC_SHAS_READY:-}" ] || return 0
  DOC_SHAS_READY=1; DOC_SHAS='{}'
  local h m one
  for h in $(harnesses); do
    m="$(snap_dir "$h")/MANIFEST.yaml"; [ -f "$m" ] || continue
    one="$(yq -o=json -I=0 '{"docs": ((.docs // {}) | map_values(.sha256))}' "$m" 2>/dev/null)" || continue
    DOC_SHAS="$(jq -c --arg h "$h" --argjson o "$one" '. + {($h): $o}' <<<"$DOC_SHAS")"
  done
}

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
