#!/usr/bin/env bash
# Steps 1 and 2 of module-leaks, shared by prepare (find-leaks.sh) and by the `subjects:` script,
# so both see the same candidates. Source after changeset.sh, modules.sh and adr.sh, with
# load_modules and load_adrs already called. Written for speed (it runs in `verify`): the
# filtering of a module's candidates is one awk, not a process per candidate.

# leak_setup — what steps 1-2 compare with: CHANGED (every selected path), EXC (the paths the ADRs
# list under `exceptions`) and, in LEAK_WORK, the path:line of every line the range adds.
leak_setup() {
  LEAK_WORK="$(mktemp -d "${TMPDIR:-/tmp}/module-leaks.XXXXXX")"; trap 'rm -rf "$LEAK_WORK"' EXIT
  added_lines | awk -F'\t' 'NF >= 2 {print $1 ":" $2}' >"$LEAK_WORK/added"
  LEAK_TREE="$(git -C "$SR_TREE" rev-parse "$(cs '.changeset.head')^{tree}")"
  CHANGED="$(cs '.changeset.files[].path')"
  EXC="$(jq -r '[.[] | .frontmatter.exceptions // [] | .[]] | .[]' <<<"$ADRS")"
  printf '%s\n' "$EXC" >"$LEAK_WORK/exc"
}

# leak_cache_dir — where a module's candidates are kept, per tree: its search (go list and a git
# grep) costs about a second a module, and the same tree is asked again by `run`, `verify` and each
# subject's prepare. Inside the repository's common git dir, never committed.
leak_cache_dir() { printf '%s/sloprail-candidates-cache' "$(git -C "$SR_TREE" rev-parse --path-format=absolute --git-common-dir)"; }

# leak_search DIR OUT — runs DIR's candidates.sh (or reads its kept output) into OUT; OUT.rc is its exit.
leak_search() {
  local dir="$1" out="$2" cache key
  cache="$(leak_cache_dir)"; key="$cache/$LEAK_TREE-$(printf '%s' "$dir" | tr '/' '_')"
  if [ -f "$key" ]; then cp "$key" "$out"; echo 0 >"$out.rc"; return 0; fi
  (cd "$SR_TREE" && "./$dir/candidates.sh") >"$out" 2>"$out.err"; echo $? >"$out.rc"
  if [ "$(cat "$out.rc")" = 0 ] && mkdir -p "$cache" 2>/dev/null; then cp "$out" "$key.$$" && mv "$key.$$" "$key"; fi
}

# leak_prefetch — searches every module that has a candidates.sh at once, in parallel (leak_left
# then reads what was found), so the time is that of the slowest search, not their sum.
leak_prefetch() {
  local m dir
  while IFS= read -r m; do
    dir="$(jq -r '.dir' <<<"$m")"
    [ -x "$SR_TREE/$dir/candidates.sh" ] || continue
    leak_search "$dir" "$LEAK_WORK/cand-$(printf '%s' "$dir" | tr '/' '_')" &
  done < <(jq -c '.[]' <<<"$MODULES")
  wait
}

# leak_left MODULE_JSON — sets LEFT to the candidates of that module still to judge, as a JSON
# array of {path, line, text}, and returns 1 when the module has no candidates.sh. 1. the module's
# own search; 2. only the lines this range adds (all of them when its module.yaml or
# candidates.sh changed), minus its home, tests and the ADRs' exceptions.
leak_left() {
  local m="$1" dir home g whole kept p l t cand
  LEFT="[]"
  dir="$(jq -r '.dir' <<<"$m")"
  [ -x "$SR_TREE/$dir/candidates.sh" ] || return 1
  home=(); while IFS= read -r g; do home+=("$g"); done < <(jq -r '.home[]' <<<"$m")
  cand="$LEAK_WORK/cand-$(printf '%s' "$dir" | tr '/' '_')"
  [ -f "$cand.rc" ] || leak_search "$dir" "$cand"
  [ "$(cat "$cand.rc")" = 0 ] ||
    refuse "$dir/candidates.sh failed, so leaks of that module cannot be found: $(head -c 300 "$cand.err")"
  whole=0; printf '%s\n' "$CHANGED" | grep -Fxq -e "$dir/module.yaml" -e "$dir/candidates.sh" && whole=1
  kept="$(awk -v whole="$whole" -v addedf="$LEAK_WORK/added" -v excf="$LEAK_WORK/exc" '
    BEGIN { while ((getline x < addedf) > 0) add[x] = 1; while ((getline x < excf) > 0) exc[x] = 1 }
    /^[[:space:]]*$/ { next }
    { if (!match($0, /^.+:[0-9]+:/)) { print "BAD\t" $0; next }
      pre = substr($0, 1, RLENGTH - 1); t = substr($0, RLENGTH + 1)
      k = match(pre, /:[0-9]+$/); p = substr(pre, 1, k - 1); l = substr(pre, k + 1)
      if (!whole && !((p ":" l) in add)) next
      if (p ~ /_test\.go$/ || (p in exc)) next
      print p "\t" l "\t" t }' "$cand")"
  ! printf '%s\n' "$kept" | grep -q '^BAD' || refuse "$dir/candidates.sh printed '$(printf '%s\n' "$kept" | sed -n 's/^BAD\t//p' | head -1)', not path:line:snippet"
  local rows=""
  while IFS=$'\t' read -r p l t; do
    [ -n "$p" ] || continue
    in_globs "$p" "${home[@]}" && continue
    rows="$rows$p"$'\t'"$l"$'\t'"$t"$'\n'
  done <<<"$kept"
  LEFT="$(printf '%s' "$rows" | jq -Rn '[inputs | split("\t") | {path: .[0], line: (.[1] | tonumber), text: (.[2:] | join("\t"))}]')"
  return 0
}
