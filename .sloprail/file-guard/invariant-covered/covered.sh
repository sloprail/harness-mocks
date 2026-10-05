#!/usr/bin/env bash
# The file's shape is file-guard/shapes' (schemas/invariant.cue). For each
# spec/invariants/<id>.yaml:
#   - ≥1 `// sr:invariant <id>` in non-test code
#   - ≥1 `// sr:proves <id>` in a *_test.go
# And back: every sr:invariant, and every sr:proves without a /harness part,
# names an invariant; sr:proves sits only in tests.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
load_spec invariants; inv="$SPEC"
load_markers invariant; impl="$MARKERS"
load_markers proves; proves="$(printf '%s\n' "$MARKERS" | awk -F'\t' 'NF && $2 !~ /\//')"

problems=""
add() { problems="${problems}- $1"$'\n'; }
is_id() { case $'\n'"$ids"$'\n' in *$'\n'"$1"$'\n'*) return 0 ;; esac; return 1; }   # ID is one of the invariants' ids
# a failed jq is a refusal, not an empty list of invariants (which would pass)
ids="$(jq -r '.[].id' <<<"$inv")" || refuse "the invariants could not be listed, so none could be checked"
list="$(jq -c '.[]' <<<"$inv")" || refuse "the invariants could not be listed, so none could be checked"
while IFS= read -r i; do
  [ -n "$i" ] || continue
  id="$(jq -r '.id' <<<"$i")" || refuse "an invariant's id could not be read, so it could not be checked"
  kebab "$id" || add "spec/invariants/$id.yaml: the file name must be kebab-case"
  # the matches are captured, not piped into `grep -q` (it quits at its first hit and can SIGPIPE the writer)
  m="$(awk -F'\t' -v id="$id" '$2 == id && $1 !~ /_test\.go$/' <<<"$impl")" || refuse "invariant '$id': its implementation markers could not be searched, so it could not be checked"
  [ -n "$m" ] || add "invariant '$id' has no implementation: mark the code that upholds it with // sr:invariant $id"
  m="$(awk -F'\t' -v id="$id" '$2 == id && $1 ~ /_test\.go$/' <<<"$proves")" || refuse "invariant '$id': its proving markers could not be searched, so it could not be checked"
  [ -n "$m" ] || add "invariant '$id' has no test: mark a test that proves it with // sr:proves $id"
done <<<"$list"
while IFS=$'\t' read -r path id; do
  [ -n "$path" ] || continue
  is_id "$id" || add "$path: sr:invariant '$id' names no spec/invariants/$id.yaml"
done <<<"$impl"
while IFS=$'\t' read -r path id; do
  [ -n "$path" ] || continue
  is_id "$id" || add "$path: sr:proves '$id' names no spec/invariants/$id.yaml"
  case "$path" in *_test.go) ;; *) add "$path: sr:proves belongs on a test, in a *_test.go" ;; esac
done <<<"$proves"
[ -z "$problems" ] && exit 0
refuse "Invariants not implemented or not proven:
${problems}"
