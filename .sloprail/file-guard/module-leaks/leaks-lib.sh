#!/usr/bin/env bash
# Steps 1 and 2 of module-leaks, shared by prepare (find-leaks.sh) and by the `subjects:` script,
# so both see the same candidates. Every lookup here refuses when it fails (the callers run these
# at top level or in `||` context, where `refuse` ends the script): a failed lookup is never an
# empty list of candidates, which would let every leak through. Source after changeset.sh, modules.sh and adr.sh, with
# load_modules and load_adrs already called. Written for speed (it runs in `verify`): the
# filtering of a module's candidates is one awk, not a process per candidate.

# leak_setup — what steps 1-2 compare with: CHANGED (every selected path), EXC (the paths the ADRs
# list under `exceptions`) and, in LEAK_WORK, the path:line of every line the range adds.
leak_setup() {
  local added head
  LEAK_WORK="$(mktemp -d "${TMPDIR:-/tmp}/module-leaks.XXXXXX")" || refuse "cannot make a directory for the candidates"
  trap 'rm -rf "$LEAK_WORK"' EXIT
  jq -e 'type == "array"' <<<"$ADRS" >/dev/null 2>&1 || refuse "the ADRs could not be loaded, so the exceptions of the modules are unknown"
  added="$(added_lines)" || refuse "the lines the range adds could not be worked out, so no candidate can be matched to them"
  printf '%s\n' "$added" | awk -F'\t' 'NF >= 2 {print $1 ":" $2}' >"$LEAK_WORK/added" || refuse "the lines the range adds could not be recorded"
  head="$(cs '.changeset.head')" || refuse "the range's head could not be read"
  LEAK_TREE="$(git -C "$SR_TREE" rev-parse "$head^{tree}")" || refuse "the tree of the range's head could not be resolved"
  CHANGED="$(cs '.changeset.files[].path')" || refuse "the changed paths could not be listed"
  EXC="$(jq -r '[.[] | .frontmatter.exceptions // [] | .[]] | .[]' <<<"$ADRS")" || refuse "the ADRs' exceptions could not be read"
  printf '%s\n' "$EXC" >"$LEAK_WORK/exc" || refuse "the ADRs' exceptions could not be recorded"
}

# leak_cache_dir — where a module's candidates are kept, per tree: its search (go list and a git
# grep) costs about a second a module, and the same tree is asked again by `run`, `verify` and each
# subject's prepare. Inside the repository's common git dir, never committed.
leak_cache_dir() { local g; g="$(git -C "$SR_TREE" rev-parse --path-format=absolute --git-common-dir)" || return 1; printf '%s/sloprail-candidates-cache' "$g"; }

# leak_search DIR OUT — runs DIR's candidates.sh (or reads its kept output) into OUT; OUT.rc is its exit.
leak_search() {
  local dir="$1" out="$2" cache key
  # (a prefetch's background job refuses silently and leaves no .rc file: leak_left then searches again
  # and refuses in the open, so there is exactly one refusal)
  cache="$(leak_cache_dir)" || refuse "the git directory of the tree could not be found, so the candidates of $dir cannot be kept or read"
  key="$cache/$LEAK_TREE-$(printf '%s' "$dir" | tr '/' '_')"
  if [ -f "$key" ]; then
    # a kept entry that cannot be copied is a failed lookup, never "the search found nothing" (rc 0, an empty file)
    cp "$key" "$out" || refuse "the kept candidates of $dir could not be read, so its leaks cannot be found"
    echo 0 >"$out.rc"; return 0
  fi
  (cd "$SR_TREE" && "./$dir/candidates.sh") >"$out" 2>"$out.err"; echo $? >"$out.rc"
  if [ "$(cat "$out.rc")" = 0 ] && mkdir -p "$cache" 2>/dev/null; then cp "$out" "$key.$$" && mv "$key.$$" "$key"; fi
}

# leak_prefetch — searches every module that has a candidates.sh at once, in parallel (leak_left
# then reads what was found), so the time is that of the slowest search, not their sum.
leak_prefetch() {
  local m dir mlist
  mlist="$(jq -c '.[]' <<<"$MODULES")" || refuse "could not list the modules, so their leaks cannot be searched for"
  while IFS= read -r m; do
    [ -n "$m" ] || continue
    dir="$(jq -r '.dir' <<<"$m")" || refuse "could not read a module's directory, so its leaks cannot be searched for"
    [ -x "$SR_TREE/$dir/candidates.sh" ] || continue
    # the job reports failure by leaving no .rc file, not by printing a refusal of its own (subjects.sh emits the one)
    ( refuse() { exit 1; }; leak_search "$dir" "$LEAK_WORK/cand-$(printf '%s' "$dir" | tr '/' '_')" ) &
  done <<<"$mlist"
  wait
}

# leak_left MODULE_JSON — sets LEFT to the candidates of that module still to judge, as a JSON
# array of {path, line, text}, and returns 1 when the module has no candidates.sh. 1. the module's
# own search; 2. only the lines this range adds (all of them when its module.yaml or
# candidates.sh changed), minus its home, tests and the ADRs' exceptions.
leak_left() {
  local m="$1" dir home g whole kept p l t cand hlist
  LEFT="[]"
  dir="$(jq -r '.dir' <<<"$m")" || refuse "could not read a module's directory, so its leaks cannot be found"
  [ -x "$SR_TREE/$dir/candidates.sh" ] || return 1
  hlist="$(jq -r '.home[]' <<<"$m")" || refuse "could not read the home of module $dir, so its leaks cannot be found"
  home=(); while IFS= read -r g; do [ -n "$g" ] && home+=("$g"); done <<<"$hlist"
  cand="$LEAK_WORK/cand-$(printf '%s' "$dir" | tr '/' '_')"
  [ -f "$cand.rc" ] || leak_search "$dir" "$cand"
  [ "$(cat "$cand.rc")" = 0 ] ||
    refuse "$dir/candidates.sh failed, so leaks of that module cannot be found: $(head -c 300 "$cand.err")"
  whole=0; grep -Fxq -e "$dir/module.yaml" -e "$dir/candidates.sh" <<<"$CHANGED" && whole=1
  kept="$(awk -v whole="$whole" -v addedf="$LEAK_WORK/added" -v excf="$LEAK_WORK/exc" '
    BEGIN { while ((getline x < addedf) > 0) add[x] = 1; while ((getline x < excf) > 0) exc[x] = 1 }
    /^[[:space:]]*$/ { next }
    { if (!match($0, /^.+:[0-9]+:/)) { print "BAD\t" $0; next }
      pre = substr($0, 1, RLENGTH - 1); t = substr($0, RLENGTH + 1)
      k = match(pre, /:[0-9]+$/); p = substr(pre, 1, k - 1); l = substr(pre, k + 1)
      if (!whole && !((p ":" l) in add)) next
      if (p ~ /_test\.go$/ || (p in exc)) next
      print p "\t" l "\t" t }' "$cand")" || refuse "the candidates of $dir could not be filtered, so its leaks cannot be found"
  ! grep -q '^BAD' <<<"$kept" || refuse "$dir/candidates.sh printed '$(printf '%s\n' "$kept" | sed -n 's/^BAD\t//p' | head -1)', not path:line:snippet"
  local rows=""
  while IFS=$'\t' read -r p l t; do
    [ -n "$p" ] || continue
    in_globs "$p" "${home[@]}" && continue
    rows="$rows$p"$'\t'"$l"$'\t'"$t"$'\n'
  done <<<"$kept"
  LEFT="$(printf '%s' "$rows" | jq -Rn '[inputs | split("\t") | {path: .[0], line: (.[1] | tonumber), text: (.[2:] | join("\t"))}]')" ||
    refuse "the candidates of $dir could not be read as path, line and text, so its leaks cannot be found"
  return 0
}
