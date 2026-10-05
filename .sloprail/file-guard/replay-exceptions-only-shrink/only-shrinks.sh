#!/usr/bin/env bash
# The notReplaying map at the head only shrinks from the base, in every replay exception list
# (one replay_allowlist_test.go per mock): a change may remove entries, never add one, and may
# not move an existing entry's reason to a weaker category (flaky: is weaker than untriaged:,
# which is weaker than any triaged reason such as adapter: or mock gap:). A file created by the
# change has no base, so it may carry no entries: every entry would be an addition.
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

# ranks TEXT: "key<TAB>rank" per entry of the map, sorted; the rank is how settled the reason is:
# 0 flaky:, 1 untriaged:, 2 any other reason
ranks() {
  printf '%s\n' "$1" | sed -n '/^var notReplaying = map\[string\]string{/,/^}/p' |
    sed -n 's/^[[:space:]]*"\([^"]*\)":[[:space:]]*"\(.*\)",\{0,1\}$/\1\t\2/p' |
    awk -F'\t' '{ r = 2; if ($2 ~ /^flaky:/) r = 0; else if ($2 ~ /^untriaged:/) r = 1; print $1 "\t" r }' | sort -u
}
# malformed TEXT: the lines of the map that are not blank, a whole-line comment or one
# "run": "reason", entry; such a line (a trailing comment, a raw string, a value on two lines)
# hides an entry from the comparison, so it is refused instead of read
malformed() {
  printf '%s\n' "$1" | sed -n '/^var notReplaying = map\[string\]string{$/,/^}$/p' | sed '1d;$d' |
    grep -vE '^[[:space:]]*($|//|"[^"]*":[[:space:]]*"([^"\\]|\\.)*",$)'
  # the category a reason starts with is read from the source text, so an escape inside it
  # ("\u0066laky:") would be read by Go as another category
  printf '%s\n' "$1" | grep -E '^[[:space:]]*"[^"]*":[[:space:]]*"[^"\\]{0,11}\\'
}
tab="$(printf '\t')"
rank_name() { case "$1" in 0) printf 'flaky:' ;; 1) printf 'untriaged:' ;; *) printf 'a triaged reason' ;; esac; }

i=0
while [ "$i" -lt "$count" ]; do
  path="$(printf '%s' "$payload" | jq -r --argjson i "$i" '.changeset.files[$i].path')" ||
    refuse "could not read file $i of the changeset, so it could not be checked"
  status="$(printf '%s' "$payload" | jq -r --argjson i "$i" '.changeset.files[$i].status')" ||
    refuse "could not read $path from the changeset, so it could not be checked"
  i=$((i + 1))
  # the generated replay test runs a flaky: entry flakyRuns (3) times and fails when none is green
  case "$path" in
    */generated_replay_test.go)
      [ "$status" = "D" ] && continue
      new="$(printf '%s' "$payload" | jq -r --argjson i "$((i - 1))" '.changeset.files[$i].newContent')" ||
        refuse "could not read $path from the changeset, so it could not be checked"
      runs="$(printf '%s\n' "$new" | sed -n 's/^const flakyRuns = \([0-9][0-9]*\)$/\1/p')"
      [ "$runs" = 3 ] || refuse "$path: a flaky: entry is replayed three times: declare 'const flakyRuns = 3'"
      printf '%s\n' "$new" | grep -q 'replayUntilGreen(run, flakyRuns)' &&
        printf '%s\n' "$new" | grep -q 'strings.HasPrefix(reason, "flaky:")' &&
        printf '%s\n' "$new" | grep -q 'case flaky && (err != nil || diff != ""):' ||
        refuse "$path: a flaky: entry must be run through replayUntilGreen(run, flakyRuns) and fail when it is never green, never skipped or passed outright"
      continue
      ;;
  esac
  # a deleted file has no list; a created one has an empty base, so each of its entries is an addition
  if [ "$status" = "D" ]; then continue; fi
  old=""
  if [ "$status" != "A" ]; then
    old="$(printf '%s' "$payload" | jq -r --argjson i "$((i - 1))" '.changeset.files[$i].oldContent')" ||
      refuse "could not read the base of $path from the changeset, so it could not be checked"
  fi
  new="$(printf '%s' "$payload" | jq -r --argjson i "$((i - 1))" '.changeset.files[$i].newContent')" ||
    refuse "could not read $path from the changeset, so it could not be checked"
  printf '%s\n' "$new" | grep -qx 'var notReplaying = map\[string\]string{' ||
    refuse "$path: the notReplaying map could not be found; keep it as 'var notReplaying = map[string]string{' with one \"run\": \"reason\" per line"
  # exactly one declaration: a second one (inside a block comment or a raw string, with the real map
  # written some other way) would show this rule a map that Go never reads
  [ "$(printf '%s\n' "$new" | grep -c '^[[:space:]]*var[[:space:]][[:space:]]*notReplaying\b')" = 1 ] ||
    refuse "$path: notReplaying must be declared exactly once, as 'var notReplaying = map[string]string{' on a line of its own"
  bad="$(malformed "$new")"
  [ -z "$bad" ] || refuse "$path: a notReplaying line is not one \"run\": \"reason\", entry, so the list could not be compared (a line this rule cannot read would hide an entry): $(printf '%s' "$bad" | head -n 1)"
  added="$(comm -13 <(keys "$old") <(keys "$new"))"
  [ -z "$added" ] || refuse "$path: the replay exception list may only shrink, and these entries were added: $(printf '%s' "$added" | tr '\n' ' '): make the run replay instead of listing it"
  weaker=""
  while IFS="$tab" read -r key was now; do
    [ -n "$key" ] || continue
    weaker="$weaker$key ($(rank_name "$was") -> $(rank_name "$now")); "
  done < <(join -t "$tab" <(ranks "$old") <(ranks "$new") | awk -F'\t' '$3 < $2')
  [ -z "$weaker" ] || refuse "$path: the replay exception list may only shrink, and these entries' reasons became weaker: ${weaker%; }: triage the entry or make the run replay, a flaky: entry counts as an addition"
done
exit 0
