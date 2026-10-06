#!/usr/bin/env bash
# The notReplaying map at the head only shrinks from the base, in every replay exception list
# (a replay_allowlist_test.go under any <mock>-mock/e2e/): a change may remove entries, never add one, and may
# not move an existing entry's reason to a weaker category (flaky: is weaker than untriaged:,
# which is weaker than any triaged reason such as adapter: or mock gap:). A file created by the
# change has no base, so it may carry no entries: every entry would be an addition. A list moved with
# git mv, to another replay directory of its mock, is a rename and is compared with the list it was.
#
# The rule compares the map's text, so it accepts only forms it can compare (one "run": "reason", per
# line, no escape inside a reason's category, the map named only by its declaration): these restrictions
# exist so that the entry comparison can be trusted. The checks of the other files and of the reason
# categories close three bypasses the rule before them passed (a reason weakened to one with no category,
# an init() in another file adding an entry, the list moved to a file of another name with a new entry). Deleting a list file outright is not judged:
# removing the whole list only shrinks it.
# Follows the skill's check-template.sh: anything but a readable Changeset is a refusal.
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

# two_line_empty — stdin with a one-line `var notReplaying = map[string]string{}` written as the declaration line and a
# closing brace line, the form the rest of the rule reads
two_line_empty() {
  awk '/^var notReplaying = map\[string\]string\{\}[ \t]*$/ { print "var notReplaying = map[string]string{"; print "}"; next } { print }'
}

tab="$(printf '\t')"
# the reasons start with one of these categories (the user's words: a reason with none of them is refused)
known='^(adapter:|mock gap:|untriaged:|flaky:)'

# entries TEXT — "key<TAB>rank<TAB>known" per entry of the map in TEXT, sorted by key; the rank is how
# settled the reason is: 0 flaky:, 1 untriaged:, 2 any other; known is 1 when the reason starts with a
# known category. Sets ENTRIES, or refuses with an error when the list has entry lines and yields none.
entries() {
  local body n
  body="$(sed -n '/^var notReplaying = map\[string\]string{/,/^}/p' <<<"$1")" || refuse_error "could not read the notReplaying map"
  ENTRIES="$(sed -n 's/^[[:space:]]*"\([^"]*\)":[[:space:]]*"\(.*\)",\{0,1\}$/\1\t\2/p' <<<"$body" |
    awk -F'\t' -v known="$known" '{ r = 2; if ($2 ~ /^flaky:/) r = 0; else if ($2 ~ /^untriaged:/) r = 1; k = ($2 ~ known) ? 1 : 0; print $1 "\t" r "\t" k }' |
    LC_ALL=C sort -t "$tab" -k1,1 -u)" || refuse_error "could not read the entries of the notReplaying map"
  n="$(grep -c '^[[:space:]]*"' <<<"$body")"
  if [ "${n:-0}" -gt 0 ] && [ -z "$ENTRIES" ]; then
    refuse_error "the notReplaying map has $n entry lines but none could be read, so it cannot be compared"
  fi
}
# malformed TEXT: the lines of the map that are not blank, a whole-line comment or one
# "run": "reason", entry; such a line (a trailing comment, a raw string, a value on two lines)
# hides an entry from the comparison, so it is refused instead of read
malformed() {
  printf '%s\n' "$1" | sed -n '/^var notReplaying = map\[string\]string{$/,/^}$/p' | sed '1d;$d' |
    grep -vE '^[[:space:]]*($|//|"[^"]*":[[:space:]]*"([^"\\]|\\.)*",$)'
  # a key with a tab would break the key/rank columns this rule compares by: refused like any line it cannot read
  printf '%s\n' "$1" | sed -n '/^var notReplaying = map\[string\]string{$/,/^}$/p' | grep -E '^[[:space:]]*"[^"]*'"$(printf '\t')"'[^"]*":'
  # the category a reason starts with is read from the source text, so an escape inside it
  # ("\u0066laky:") would be read by Go as another category
  printf '%s\n' "$1" | grep -E '^[[:space:]]*"[^"]*":[[:space:]]*"[^"\\]{0,11}\\'
}
rank_name() { case "$1" in 0) printf 'flaky:' ;; 1) printf 'untriaged:' ;; *) printf 'a triaged reason' ;; esac; }

i=0
while [ "$i" -lt "$count" ]; do
  path="$(printf '%s' "$payload" | jq -r --argjson i "$i" '.changeset.files[$i].path')" ||
    refuse_error "could not read file $i of the changeset, so it could not be checked"
  status="$(printf '%s' "$payload" | jq -r --argjson i "$i" '.changeset.files[$i].status')" ||
    refuse_error "could not read $path from the changeset, so it could not be checked"
  i=$((i + 1))
  # only a list file has a base to compare. Of the other files, only the generated replay test may name
  # the list: a list moved to a file of another name, or an init() in any other file, would hide entries.
  case "$path" in
    */replay_allowlist_test.go) ;;
    */generated_replay_test.go)
      # the generated test may keep reading the list, never gain a line that names it: such a line could be a
      # write (an init() adding an entry, a delete(, a maps.Copy(), so each line naming it must already be in the old file
      [ "$status" = "D" ] && continue
      gnew="$(printf '%s' "$payload" | jq -r --argjson i "$((i - 1))" '.changeset.files[$i].newContent')" ||
        refuse_error "could not read $path from the changeset, so it could not be checked"
      gold=""
      if [ "$status" != "A" ]; then
        gold="$(printf '%s' "$payload" | jq -r --argjson i "$((i - 1))" '.changeset.files[$i].oldContent')" ||
          refuse_error "could not read the base of $path from the changeset, so it could not be checked"
      fi
      gnew_lines="$(grep -v '^[[:space:]]*//' <<<"$gnew" | grep -w notReplaying | sed 's/^[[:space:]]*//' | LC_ALL=C sort -u)"
      gold_lines="$(grep -v '^[[:space:]]*//' <<<"$gold" | grep -w notReplaying | sed 's/^[[:space:]]*//' | LC_ALL=C sort -u)"
      gadded="$(LC_ALL=C comm -23 <(printf '%s\n' "$gnew_lines") <(printf '%s\n' "$gold_lines") | grep -v '^$')"
      [ -z "$gadded" ] ||
        refuse "$path: the generated test names notReplaying in a line that was not there before (a write, a delete( or a maps.Copy( would add an entry unseen): $(printf '%s' "$gadded" | head -n 1)"
      continue ;;
    *.go)
      [ "$status" = "D" ] && continue
      other="$(printf '%s' "$payload" | jq -r --argjson i "$((i - 1))" '.changeset.files[$i].newContent')" ||
        refuse_error "could not read $path from the changeset, so it could not be checked"
      if grep -v '^[[:space:]]*//' <<<"$other" | grep -qw notReplaying; then
        refuse "$path: notReplaying is named outside replay_allowlist_test.go and generated_replay_test.go: an entry added or moved there would not be seen, so keep the list, and every edit of it, in replay_allowlist_test.go"
      fi
      continue ;;
    *) continue ;;
  esac
  # a deleted file has no list. A created one has an empty base, so each of its entries is an addition
  # (a list moved with git mv is a rename: its base is the list it was).
  if [ "$status" = "D" ]; then continue; fi
  old=""
  if [ "$status" != "A" ]; then
    old="$(printf '%s' "$payload" | jq -r --argjson i "$((i - 1))" '.changeset.files[$i].oldContent')" ||
      refuse_error "could not read the base of $path from the changeset, so it could not be checked"
  fi
  new="$(printf '%s' "$payload" | jq -r --argjson i "$((i - 1))" '.changeset.files[$i].newContent')" ||
    refuse_error "could not read $path from the changeset, so it could not be checked"
  # the empty map gofmt writes on one line is the same empty list as the two-line form
  old="$(two_line_empty <<<"$old")" || refuse_error "could not read the base of $path, so it could not be checked"
  new="$(two_line_empty <<<"$new")" || refuse_error "could not read $path, so it could not be checked"
  grep -qx 'var notReplaying = map\[string\]string{' <<<"$new" ||
    refuse "$path: the notReplaying map could not be found; keep it as 'var notReplaying = map[string]string{' with one \"run\": \"reason\" per line"
  # exactly one declaration: a second one (inside a block comment or a raw string, with the real map
  # written some other way) would show this rule a map that Go never reads
  [ "$(printf '%s\n' "$new" | grep -c '^[[:space:]]*var[[:space:]][[:space:]]*notReplaying\b')" = 1 ] ||
    refuse "$path: notReplaying must be declared exactly once, as 'var notReplaying = map[string]string{' on a line of its own"
  # notReplaying appears on the declaration line only (and in whole-line comments): a read or write of
  # the map elsewhere (an init() adding an entry, a var ( group) is an entry this rule would not see
  [ "$(printf '%s\n' "$new" | grep -v '^[[:space:]]*//' | grep -c 'notReplaying')" = 1 ] ||
    refuse "$path: notReplaying may appear only on its declaration line (comments aside): do not read or change the map elsewhere in this file"
  bad="$(malformed "$new")"
  [ -z "$bad" ] || refuse "$path: a notReplaying line is not one \"run\": \"reason\", entry, so the list could not be compared (a line this rule cannot read would hide an entry): $(printf '%s' "$bad" | head -n 1)"
  entries "$old"; old_entries="$ENTRIES"
  entries "$new"; new_entries="$ENTRIES"
  added="$(LC_ALL=C comm -13 <(cut -f1 <<<"$old_entries") <(cut -f1 <<<"$new_entries"))"
  [ -z "$added" ] || refuse "$path: the replay exception list may only shrink, and these entries were added: $(printf '%s' "$added" | tr '\n' ' '): make the run replay instead of listing it"
  weaker=""
  unknown=""
  while IFS="$tab" read -r key was was_known now now_known; do
    [ -n "$key" ] || continue
    [ "$now" -ge "$was" ] || weaker="$weaker$key ($(rank_name "$was") -> $(rank_name "$now")); "
    [ "$now_known" -eq 1 ] || unknown="$unknown$key; "
  done < <(LC_ALL=C join -t "$tab" <(printf '%s\n' "$old_entries") <(printf '%s\n' "$new_entries"))
  [ -z "$unknown" ] || refuse "$path: the reasons of these entries do not start with a known category (adapter:, mock gap:, untriaged: or flaky:): ${unknown%; }"
  [ -z "$weaker" ] || refuse "$path: the replay exception list may only shrink, and these entries' reasons became weaker: ${weaker%; }: triage the entry or make the run replay, a flaky: entry counts as an addition"
done
exit 0
