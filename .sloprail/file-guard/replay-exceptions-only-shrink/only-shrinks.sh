#!/usr/bin/env bash
# The notReplaying map's keys at the head are among its keys at the base: a change may remove
# entries, never add one. A file created by the change (no base) is the list's first version.
# Follows the skill's check-template.sh: anything but a readable Changeset is a refusal.
set -uo pipefail

payload="$(cat)"

refuse() {
  jq -n --arg r "$1" '{reason: $r}'
  exit 1
}

[ "$(printf '%s' "$payload" | jq -r '.event.kind // ""')" = "Changeset" ] ||
  refuse "expected a Changeset event, so the changed files could not be checked"
printf '%s' "$payload" | jq -e '.changeset.files | type == "array"' >/dev/null 2>&1 ||
  refuse "the changeset's files could not be read, so they could not be checked"
count="$(printf '%s' "$payload" | jq -r '.changeset.files | length')" || count=""
case "$count" in '' | *[!0-9]*) refuse "the changeset's files could not be read, so they could not be checked" ;; esac

# keys TEXT: the keys of the notReplaying map literal in a Go source, one per line
keys() { printf '%s\n' "$1" | sed -n '/^var notReplaying = map\[string\]string{/,/^}/p' | sed -n 's/^[[:space:]]*"\([^"]*\)":.*/\1/p' | sort -u; }

i=0
while [ "$i" -lt "$count" ]; do
  path="$(printf '%s' "$payload" | jq -r --argjson i "$i" '.changeset.files[$i].path')" ||
    refuse "could not read file $i of the changeset, so it could not be checked"
  status="$(printf '%s' "$payload" | jq -r --argjson i "$i" '.changeset.files[$i].status')" ||
    refuse "could not read $path from the changeset, so it could not be checked"
  i=$((i + 1))
  # a deleted file has no list; a created one is the list's first version
  if [ "$status" = "D" ] || [ "$status" = "A" ]; then continue; fi
  old="$(printf '%s' "$payload" | jq -r --argjson i "$((i - 1))" '.changeset.files[$i].oldContent')" ||
    refuse "could not read the base of $path from the changeset, so it could not be checked"
  new="$(printf '%s' "$payload" | jq -r --argjson i "$((i - 1))" '.changeset.files[$i].newContent')" ||
    refuse "could not read $path from the changeset, so it could not be checked"
  printf '%s\n' "$new" | grep -q '^var notReplaying = map\[string\]string{' ||
    refuse "$path: the notReplaying map could not be found; keep it as 'var notReplaying = map[string]string{' with one \"run\": \"reason\" per line"
  added="$(comm -13 <(keys "$old") <(keys "$new"))"
  [ -z "$added" ] || refuse "$path: the replay exception list may only shrink, and these entries were added: $(printf '%s' "$added" | tr '\n' ' '): make the run replay instead of listing it"
done
exit 0
