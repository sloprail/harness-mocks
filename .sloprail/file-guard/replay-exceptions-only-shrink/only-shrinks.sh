#!/usr/bin/env bash
# The notReplaying map at the head only shrinks from the base, in every replay exception list (one
# replay_allowlist_test.go per mock): a change may remove entries, never add one, and may not move an
# existing entry's reason to a weaker category (flaky: is weaker than untriaged:, which is weaker than
# any triaged reason such as adapter: or mock gap:). A file created by the change has no base, so it may
# carry no entries: every entry would be an addition.
#
# The Go is read from its syntax tree and its type information by tools/replaycheck, never from its
# text: the map is read as the declaration it is (an escape, a raw string, a one-line map or a comment
# cannot hide an entry), the generated replay test may only read the list, flakyRuns is the one
# package-level const 3, and no other file of the package may name the list. replayUntilGreen and
# TestGeneratedReplay, which repeat a flaky: entry and fail it when it is never green, are pinned: printed
# without comments they must equal the canonical copies in canonical/ byte for byte. Changing a canonical
# copy changes the rule, and needs the user's citation.
# Deleting a list file outright is not judged: removing the whole list only shrinks it, provided
# nothing that still reads notReplaying is left without its list file (replaycheck check).
# Anything the checker cannot do (a failed build, a failed run) is refuse_error, not a verdict.
set -uo pipefail

payload="$(cat)"

refuse() {
  jq -n --arg r "$1" '{reason: $r}'
  exit 1
}

# refuse_error MESSAGE — the tooling failed, not the change: still refused, but "error": true makes the
# engine store no verdict, so the next run tries again (see _lib/changeset.sh)
refuse_error() {
  jq -n --arg r "$1" '{reason: $r, error: true}'
  exit 1
}

[ "$(printf '%s' "$payload" | jq -r '.event.kind // ""')" = "Changeset" ] ||
  refuse_error "expected a Changeset event, so the changed files could not be checked"
printf '%s' "$payload" | jq -e '.changeset.files | type == "array"' >/dev/null 2>&1 ||
  refuse_error "the changeset's files could not be read, so they could not be checked"
count="$(printf '%s' "$payload" | jq -r '.changeset.files | length')" || count=""
case "$count" in '' | *[!0-9]*) refuse_error "the changeset's files could not be read, so they could not be checked" ;; esac

[ -d "${SR_TREE:-}/tools/replaycheck" ] ||
  refuse_error "tools/replaycheck is not in the committed tree, so the replay exception lists cannot be checked"
work="$(mktemp -d)" || refuse_error "could not make a directory for the replay checker"
bin="$work/replaycheck"
canon="$(cd "${SR_GUARDRAIL_DIR:-.}" && pwd)/canonical"
[ -d "$canon" ] || refuse_error "the canonical copies are not in the rule's folder ($canon), so the generated replay tests cannot be checked"
build="$(cd "$SR_TREE" && go build -o "$bin" ./tools/replaycheck 2>&1)" ||
  refuse_error "could not build tools/replaycheck, so the replay exception lists cannot be checked: $build"

tab="$(printf '\t')"
rank_name() { case "$1" in 0) printf 'flaky:' ;; 1) printf 'untriaged:' ;; *) printf 'a triaged reason' ;; esac; }

# checker ARGS... [< stdin] — runs replaycheck; sets out (its stdout) and rc. Exit 2 or anything else
# than 0 and 1 is the tool failing: an error, not a verdict.
checker() {
  out="$("$bin" "$@" 2>"$work/err")"
  rc=$?
  case "$rc" in 0 | 1) ;; *) refuse_error "replaycheck failed ($rc), so the replay exception lists cannot be checked: $(cat "$work/err")" ;; esac
}

# entries TEXT — sets ENTRIES to the sorted "key<TAB>rank" lines of the list in TEXT; a list that
# violates the structure is a refusal naming what is wrong
entries() {
  out="$(printf '%s\n' "$1" | "$bin" entries 2>"$work/err")"
  rc=$?
  case "$rc" in
    0) ENTRIES="$(printf '%s\n' "$out" | LC_ALL=C sort -t "$tab" -k1,1)" ;;
    1) ENTRIES_BAD="$out" ;;
    *) refuse_error "replaycheck failed ($rc), so the replay exception lists cannot be checked: $(cat "$work/err")" ;;
  esac
  return "$rc"
}

i=0
while [ "$i" -lt "$count" ]; do
  path="$(printf '%s' "$payload" | jq -r --argjson i "$i" '.changeset.files[$i].path')" ||
    refuse_error "could not read file $i of the changeset, so it could not be checked"
  status="$(printf '%s' "$payload" | jq -r --argjson i "$i" '.changeset.files[$i].status')" ||
    refuse_error "could not read $path from the changeset, so it could not be checked"
  i=$((i + 1))
  # only a list file has a base to compare; the other files are judged by the package check below
  case "$path" in */replay_allowlist_test.go) ;; *) continue ;; esac
  [ "$status" = "D" ] && continue
  old=""
  if [ "$status" != "A" ]; then
    old="$(printf '%s' "$payload" | jq -r --argjson i "$((i - 1))" '.changeset.files[$i].oldContent')" ||
      refuse_error "could not read the base of $path from the changeset, so it could not be checked"
  fi
  new="$(printf '%s' "$payload" | jq -r --argjson i "$((i - 1))" '.changeset.files[$i].newContent')" ||
    refuse_error "could not read $path from the changeset, so it could not be checked"
  ENTRIES="" ENTRIES_BAD=""
  if [ -n "$old" ]; then
    entries "$old" || refuse_error "$path: the list at the base could not be read (its structure is not one this rule reads), so it could not be compared with the head: $ENTRIES_BAD"
    old_entries="$ENTRIES"
  else
    old_entries=""
  fi
  entries "$new" || refuse "$path: the list is not in a form this rule can compare, so a change to it cannot be trusted: $ENTRIES_BAD"
  new_entries="$ENTRIES"
  added="$(comm -13 <(printf '%s\n' "$old_entries" | cut -f1 | LC_ALL=C sort -u) <(printf '%s\n' "$new_entries" | cut -f1 | LC_ALL=C sort -u))"
  [ -z "$added" ] || refuse "$path: the replay exception list may only shrink, and these entries were added: $(printf '%s' "$added" | tr '\n' ' '): make the run replay instead of listing it"
  weaker=""
  while IFS="$tab" read -r key was now; do
    [ -n "$key" ] || continue
    weaker="$weaker$key ($(rank_name "$was") -> $(rank_name "$now")); "
  done < <(join -t "$tab" <(printf '%s\n' "$old_entries" | LC_ALL=C sort -t "$tab" -k1,1) <(printf '%s\n' "$new_entries" | LC_ALL=C sort -t "$tab" -k1,1) | awk -F'\t' '$3 < $2')
  [ -z "$weaker" ] || refuse "$path: the replay exception list may only shrink, and these entries' reasons became weaker: ${weaker%; }: triage the entry or make the run replay, a flaky: entry counts as an addition"
done

# the package check: every directory with a list file, a generated test, or any file naming the list
files="$(git -C "$SR_TREE" ls-files -- '*replay_allowlist_test.go' '*generated_replay_test.go' 2>&1)" ||
  refuse_error "could not list the replay files of the committed tree: $files"
mentions="$(git -C "$SR_TREE" grep -l -w notReplaying -- '*.go' 2>&1)"
rc=$?
[ "$rc" -le 1 ] || refuse_error "could not search the committed tree for notReplaying: $mentions"
[ "$rc" -eq 0 ] || mentions=""
dirs="$(printf '%s\n%s\n' "$files" "$mentions" | grep -v '^$' | while IFS= read -r f; do dirname "$f"; done | LC_ALL=C sort -u)"
bad=""
for d in $dirs; do
  checker check "$SR_TREE/$d" "$canon"
  [ "$rc" -eq 0 ] || bad="$bad$out"$'\n'
done
[ -z "$bad" ] || refuse "the replay exception list and the generated replay test are not as adr/replay-exceptions-only-shrink needs:
${bad//$SR_TREE\//}"
exit 0
