#!/usr/bin/env bash
# The file's shape (statement, cells, citation formats, a run under its own
# harness) is file-guard/shapes' (schemas/capability.cue). Across files, for
# each spec/capabilities/<id>.yaml:
#   - a cell for EVERY harness mock (<h>-mock/ dirs); none for a harness that
#     does not exist
#   - exactly one `// sr:capability <id>`, under internal/
#   - each supported cell ({docs, runs}): ≥1 `// sr:proves <id>/<h>` in a *_test.go
#     (its `// sr:provides <id>/<h>` adapter, both ways, is file-guard/capability-reconciled's)
#   - a cell that is not supported is one of: {supported: false, reason, docs}
#     (the harness lacks it; absence needs evidence, so a bare `false` is
#     refused; the evidence is docs and/or recorded runs, at least one) or
#     "pending" (not mocked yet). Neither is coverage: no marker
#     may name it. Pending is allowed and does not block; it is listed on stderr.
# And back: every sr:capability / sr:proves <x>/<h> names a capability, and a
# harness whose cell is supported.
# Judged per harness: one harness's missing tests never refuse a change that touches only another harness.
# Only what the change touches is checked: a capability's own shape (its file name, its one sr:capability, a cell
# for every harness mock) when the capability is touched, a harness's cell content (its tests, its deviations'
# ADRs, its evidence) when that (capability, harness) pair is touched (its cell, its recordings or its markers
# changed; the shared statement touches every pair), and the markers a change touched.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/cells.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/snapshots.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/touched.sh"
load_spec capabilities; caps="$SPEC"
load_markers capability; impl="$MARKERS"
load_markers proves; proves="$(printf '%s\n' "$MARKERS" | awk -F'\t' 'NF && $2 ~ /\//')"
# The harness mocks are listed here, by a glob of directories: harnesses() (spec.sh) ends on its last `[ -d ]`,
# so a stray file named like a mock sorted last would fail its listing. No mock directory at all is one refusal.
hs=""
for d in "$SR_TREE"/*-mock/; do
  [ -d "$d" ] || continue
  d="${d%/}"; d="${d##*/}"
  hs="${hs}${d%-mock}"$'\n'
done
[ -n "$hs" ] || refuse_error "no *-mock/ directory in the committed tree at $SR_TREE, so no harness cell could be checked (an incomplete tree?)"

load_touched
load_touched_markers || refuse_error "the capability markers this change touches could not be worked out, so nothing could be checked"
changed="$(cs '.changeset.files[].path')" || refuse_error "the changed files could not be listed, so nothing could be checked"

# runs_changed CELL — a recording the cell cites changed, or an ADR one of its deviations cites (changed or deleted)
runs_changed() {
  local r a
  for r in $(jq -r '.runs[]?' <<<"$1" 2>/dev/null); do
    awk -v r="$r/" 'index($0, r) == 1 {f = 1} END {exit !f}' <<<"$changed" && return 0
  done
  for a in $(jq -r '.deviations[]?.adr' <<<"$1" 2>/dev/null); do
    grep -qxF "adr/$a/ADR.md" <<<"$changed" && return 0
  done
  return 1
}
# in_scope_pair ID H CELL — this change touches the (capability, harness) pair: its cell changed (the shared
# statement, or the whole file, touches every pair), a marker naming the pair changed, or a recording it cites changed
in_scope_pair() {
  local th
  th="$(touched_harnesses "spec/capabilities/$1.yaml")"
  grep -qxF '*' <<<"$th" && return 0
  grep -qxF "$2" <<<"$th" && return 0
  awk -F'\t' -v f="$1/$2" '$3 == f {m = 1} END {exit !m}' <<<"$TOUCHED_MARKERS_TSV" && return 0
  runs_changed "$3"
}
# in_scope_cap ID CAPABILITY-JSON — the change touches the capability at all
in_scope_cap() {
  [ -n "$(touched_harnesses "spec/capabilities/$1.yaml")" ] && return 0
  awk -F'\t' -v id="$1" '$3 == id || index($3, id "/") == 1 {m = 1} END {exit !m}' <<<"$TOUCHED_MARKERS_TSV" && return 0
  local cell
  while IFS= read -r cell; do
    [ -n "$cell" ] && runs_changed "$cell" && return 0
  done < <(jq -c '.doc.providers | if type == "object" then to_entries[] | .value | select(type == "object") else empty end' <<<"$2" 2>/dev/null)
  return 1
}

problems=""
pending=""
add() { problems="${problems}- $1"$'\n'; }
# `// "missing"` would read a false cell as missing: jq's // treats false as absent.
cell() { jq -c --arg id "$1" --arg h "$2" '[.[] | select(.id == $id)][0].doc.providers | if type == "object" and has($h) then .[$h] else "missing" end' <<<"$caps"; }

# a failed listing is a refusal, never an empty loop that checks nothing
list="$(jq -c '.[]' <<<"$caps")" || refuse_error "the capability files could not be listed, so nothing could be checked"
while IFS= read -r c; do
  [ -n "$c" ] || continue
  id="$(jq -r '.id' <<<"$c")" || refuse_error "a capability's id could not be read, so it could not be checked"
  in_scope_cap "$id" "$c" || continue   # this change does not touch the capability
  kebab "$id" || add "spec/capabilities/$id.yaml: the file name must be kebab-case"
  jq -e '(.doc.providers | type) == "object"' <<<"$c" >/dev/null; rc=$?
  [ "$rc" -le 1 ] || refuse_error "capability '$id': its cells could not be read, so it could not be checked"
  [ "$rc" -eq 0 ] || continue   # not an object: a bad shape is shapes' finding
  keys="$(jq -r '.doc.providers | keys[]' <<<"$c")" || refuse_error "capability '$id': its cells could not be listed, so it could not be checked"
  for h in $keys; do
    case $'\n'"$hs" in *$'\n'"$h"$'\n'*) ;; *) add "capability '$id' has a cell for '$h', but there is no $h-mock/" ;; esac
  done
  n="$(printf '%s\n' "$impl" | awk -F'\t' -v id="$id" '$2 == id' | grep -c .)"
  [ "$n" -eq 1 ] || add "capability '$id' needs exactly one // sr:capability $id, in internal/ (found $n)"
  for h in $hs; do
    v="$(cell "$id" "$h")" || refuse_error "capability '$id' × '$h': its cell could not be read, so it could not be checked"
    # a missing cell is the capability's own gap; the content of a harness's cell is judged when the pair is touched
    [ "$v" = '"missing"' ] || in_scope_pair "$id" "$h" "$v" || continue
    case "$v" in
      '"missing"') add "capability '$id' has no cell for '$h': set it to {docs, runs}, {supported: false, reason, docs}, or \"pending\"" ;;
      '"pending"') pending="${pending}${id}/${h}"$'\n' ;;
      false) add "capability '$id' × '$h' is a bare false: absence needs evidence. Set {supported: false, reason: <one line>, docs: [<URL#anchor showing it absent>] and/or runs: [<recorded run showing it absent>]}, or \"pending\" if it is just not mocked yet" ;;
      *)
        if [ "$(cell_kind "$v")" = unsupported ]; then
          jq -e '(.reason | type == "string" and test("\\S") and (test("\n") | not)) and (((.docs // []) | length) + ((.runs // []) | length) > 0)' <<<"$v" >/dev/null ||
            add "capability '$id' × '$h' is {supported: false} without a one-line reason and at least one doc or recorded run that shows the feature absent"
          continue
        fi
        adrs="$(jq -r '.deviations[]?.adr' <<<"$v")" || refuse_error "capability '$id' × '$h': its deviations could not be read, so they could not be checked"
        for a in $adrs; do
          [ -f "$SR_TREE/adr/$a/ADR.md" ] || add "capability '$id' × '$h' deviates citing adr/$a, which does not exist"
        done
        printf '%s\n' "$proves" | awk -F'\t' -v f="$id/$h" '$2 == f && $1 ~ /_test\.go$/ {ok = 1} END {exit !ok}' ||
          add "capability '$id' is provided by '$h' but no test carries // sr:proves $id/$h"
        ;;
    esac
  done
done <<<"$list"

# the back checks are of the markers this change touched
touched_impl="$(awk -F'\t' 'NR == FNR { if ($2 == "capability") t[$1 "\t" $3] = 1; next } (($1 "\t" $2) in t)' <(printf '%s\n' "$TOUCHED_MARKERS_TSV") <(printf '%s\n' "$impl"))" ||
  refuse_error "the touched sr:capability markers could not be selected, so they could not be checked"
touched_proves="$(awk -F'\t' 'NR == FNR { if ($2 == "proves") t[$1 "\t" $3] = 1; next } (($1 "\t" $2) in t)' <(printf '%s\n' "$TOUCHED_MARKERS_TSV") <(printf '%s\n' "$proves"))" ||
  refuse_error "the touched sr:proves markers could not be selected, so they could not be checked"
while IFS=$'\t' read -r path id; do
  [ -n "$path" ] || continue
  jq -e --arg id "$id" 'any(.[]; .id == $id)' <<<"$caps" >/dev/null || add "$path: sr:capability '$id' names no spec/capabilities/$id.yaml"
  case "$path" in internal/*) ;; *) add "$path: capability '$id' is implemented outside internal/" ;; esac
done <<<"$touched_impl"
check_ref() {   # KIND PATH FQN WHERE-GLOB
  local kind="$1" path="$2" fqn="$3" id="${3%%/*}" h="${3#*/}" v
  case "$fqn" in */*) ;; *) add "$path: sr:$kind '$fqn' must be <capability>/<harness>"; return ;; esac
  v="$(cell "$id" "$h")" || refuse_error "$path: the cell '$id' × '$h' could not be read, so sr:$kind $fqn could not be checked"
  [ "$(cell_kind "$v")" = supported ] ||
    { add "$path: sr:$kind $fqn, but '$id' has no supported cell for '$h' ({docs, runs}); a pending or unsupported cell is not coverage"; return; }
  case "$kind:$path" in
    proves:*_test.go) ;; proves:*) add "$path: sr:proves belongs on a test, in a *_test.go" ;;
  esac
}
while IFS=$'\t' read -r path fqn; do [ -n "$path" ] && check_ref proves "$path" "$fqn"; done <<<"$touched_proves"

[ -z "$pending" ] || printf 'pending (not mocked yet, not coverage):\n%s' "$pending" >&2
[ -z "$problems" ] && exit 0
refuse "Capabilities not implemented, adapted or proven (adr/capability-once):
${problems}"
