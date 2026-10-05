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
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/cells.sh"
load_spec capabilities; caps="$SPEC"
load_markers capability; impl="$MARKERS"
load_markers proves; proves="$(printf '%s\n' "$MARKERS" | awk -F'\t' 'NF && $2 ~ /\//')"
hs="$(harnesses)"

problems=""
pending=""
add() { problems="${problems}- $1"$'\n'; }
# `// "missing"` would read a false cell as missing: jq's // treats false as absent.
cell() { jq -c --arg id "$1" --arg h "$2" '[.[] | select(.id == $id)][0].doc.providers | if has($h) then .[$h] else "missing" end' <<<"$caps"; }

while IFS= read -r c; do
  [ -n "$c" ] || continue
  id="$(jq -r '.id' <<<"$c")"
  kebab "$id" || add "spec/capabilities/$id.yaml: the file name must be kebab-case"
  jq -e '(.doc.providers | type) == "object"' <<<"$c" >/dev/null || continue   # a bad shape is shapes' finding
  for h in $(jq -r '.doc.providers | keys[]' <<<"$c"); do
    printf '%s\n' "$hs" | grep -Fxq -- "$h" || add "capability '$id' has a cell for '$h', but there is no $h-mock/"
  done
  n="$(printf '%s\n' "$impl" | awk -F'\t' -v id="$id" '$2 == id' | grep -c .)"
  [ "$n" -eq 1 ] || add "capability '$id' needs exactly one // sr:capability $id, in internal/ (found $n)"
  for h in $hs; do
    v="$(cell "$id" "$h")"
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
        for a in $(jq -r '.deviations[]?.adr' <<<"$v"); do
          [ -f "$SR_TREE/adr/$a/ADR.md" ] || add "capability '$id' × '$h' deviates citing adr/$a, which does not exist"
        done
        printf '%s\n' "$proves" | awk -F'\t' -v f="$id/$h" '$2 == f && $1 ~ /_test\.go$/' | grep -q . ||
          add "capability '$id' is provided by '$h' but no test carries // sr:proves $id/$h"
        ;;
    esac
  done
done < <(jq -c '.[]' <<<"$caps")

while IFS=$'\t' read -r path id; do
  [ -n "$path" ] || continue
  jq -e --arg id "$id" 'any(.[]; .id == $id)' <<<"$caps" >/dev/null || add "$path: sr:capability '$id' names no spec/capabilities/$id.yaml"
  case "$path" in internal/*) ;; *) add "$path: capability '$id' is implemented outside internal/" ;; esac
done <<<"$impl"
check_ref() {   # KIND PATH FQN WHERE-GLOB
  local kind="$1" path="$2" fqn="$3" id="${3%%/*}" h="${3#*/}" v
  case "$fqn" in */*) ;; *) add "$path: sr:$kind '$fqn' must be <capability>/<harness>"; return ;; esac
  v="$(cell "$id" "$h")"
  [ "$(cell_kind "$v")" = supported ] ||
    { add "$path: sr:$kind $fqn, but '$id' has no supported cell for '$h' ({docs, runs}); a pending or unsupported cell is not coverage"; return; }
  case "$kind:$path" in
    proves:*_test.go) ;; proves:*) add "$path: sr:proves belongs on a test, in a *_test.go" ;;
  esac
}
while IFS=$'\t' read -r path fqn; do [ -n "$path" ] && check_ref proves "$path" "$fqn"; done <<<"$proves"

[ -z "$pending" ] || printf 'pending (not mocked yet, not coverage):\n%s' "$pending" >&2
[ -z "$problems" ] && exit 0
refuse "Capabilities not implemented, adapted or proven (adr/capability-once):
${problems}"
