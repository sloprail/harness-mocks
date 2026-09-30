#!/usr/bin/env bash
# Per harness with snapshots, against its MANIFEST.yaml `version`:
#   script <harness>-mock/snapshots/capture.sh exists: the only writer
#   docs   every docs/<page> is registered in MANIFEST.docs, with version ==
#          version and a sha256 that matches the file (a hand edit breaks it)
#   runs   kebab name; run.yaml version == version; ≥1 sample; sample dirs are
#          UTC timestamps YYYYMMDD-HHMMSS; each has events.jsonl; no two samples
#          have identical events (a re-run that changed nothing adds nothing);
#          each sample's SEAL lists exactly its files, with matching sha256s
# Per capability cell: every cited doc URL is copied in MANIFEST.docs and its
# #anchor resolves to a heading in that copy; every cited run path exists. A run no capability cites fails.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/snapshots.sh"
load_spec capabilities; caps="$SPEC"

problems=""
add() { problems="${problems}- $1"$'\n'; }
hash() { shasum -a 256 "$1" | cut -d' ' -f1; }

for h in $(harnesses); do
  d="$(snap_dir "$h")"
  cited="$(jq -c --arg h "$h" '[.[] | .doc.providers[$h] // false | select(type == "object")]' <<<"$caps")"
  if [ ! -d "$d" ]; then
    [ "$(jq 'length' <<<"$cited")" -eq 0 ] || add "$h-mock/snapshots/ is missing, but capabilities cite snapshots for '$h'"
    continue
  fi
  m="$(yq -o=json '.' "$d/MANIFEST.yaml" 2>/dev/null)" || { add "$h-mock/snapshots/MANIFEST.yaml is missing or not valid YAML"; continue; }
  ver="$(jq -r '.version // ""' <<<"$m")"
  [ -n "$ver" ] || { add "$h-mock/snapshots/MANIFEST.yaml has no version"; continue; }
  [ -f "$d/capture.sh" ] || add "$h-mock/snapshots/capture.sh is missing: snapshots are only written by it"

  for f in "$d"/docs/*; do
    [ -f "$f" ] || continue
    p="$(basename "$f")"
    dv="$(jq -r --arg p "$p" '.docs[$p].version // ""' <<<"$m")"
    [ -n "$dv" ] || { add "$h-mock/snapshots/docs/$p is not registered in MANIFEST.docs"; continue; }
    [ "$dv" = "$ver" ] || add "$h-mock/snapshots/docs/$p was copied at $dv, not $ver: re-fetch it"
    [ "$(jq -r --arg p "$p" '.docs[$p].sha256 // ""' <<<"$m")" = "$(hash "$f")" ] ||
      add "$h-mock/snapshots/docs/$p does not match the sha256 capture.sh recorded: it was edited by hand; re-fetch it with capture.sh doc"
  done
  for p in $(jq -r '(.docs // {}) | keys[]' <<<"$m"); do
    [ -f "$d/docs/$p" ] || add "MANIFEST.docs registers $p, but $h-mock/snapshots/docs/$p does not exist"
  done

  for r in "$d"/runs/*/; do
    [ -d "$r" ] || continue
    name="$(basename "$r")"
    kebab "$name" || add "$h-mock/snapshots/runs/$name: the name must be kebab-case"
    rv="$(yq -r '.version // ""' "$r/run.yaml" 2>/dev/null)"
    [ "$rv" = "$ver" ] || add "run $h/$name was captured at '${rv:-?}', not $ver: re-capture it"
    seen="" n=0
    for s in "$r"/samples/*/; do
      [ -d "$s" ] || continue
      n=$((n + 1)); ts="$(basename "$s")"
      printf '%s' "$ts" | grep -Eq '^[0-9]{8}-[0-9]{6}$' || add "run $h/$name: sample '$ts' must be named YYYYMMDD-HHMMSS (UTC)"
      [ -f "$s/events.jsonl" ] || { add "run $h/$name sample $ts has no events.jsonl"; continue; }
      if [ ! -f "$s/SEAL" ]; then add "run $h/$name sample $ts has no SEAL: it was not written by capture.sh"
      else
        listed="$(awk '{print $2}' "$s/SEAL" | LC_ALL=C sort)"
        actual="$(cd "$s" && find . -type f ! -name SEAL | LC_ALL=C sort)"
        [ "$listed" = "$actual" ] && (cd "$s" && shasum -a 256 -c SEAL >/dev/null 2>&1) ||
          add "run $h/$name sample $ts does not match its SEAL: it was edited by hand; re-capture it with capture.sh run $name"
      fi
      hv="$(hash "$s/events.jsonl")"
      dup="$(printf '%s\n' "$seen" | awk -v h="$hv" '$1 == h {print $2}')"
      [ -z "$dup" ] || add "run $h/$name: sample $ts has the same events as $dup; drop it"
      seen="$seen$hv $ts"$'\n'
    done
    [ "$n" -gt 0 ] || add "run $h/$name has no samples"
    jq -e --arg n "$h-mock/snapshots/runs/$name" 'any(.[]; .runs | index($n))' <<<"$cited" >/dev/null ||
      add "run $h/$name is cited by no capability: cite it, or delete it"
  done

  while IFS=$'\t' read -r id ref kind; do
    [ -n "$id" ] || continue
    if [ "$kind" = run ]; then
      [ -d "$SR_TREE/$ref" ] || add "capability '$id' cites run '$ref', which does not exist"
    else
      doc_file "$h" "$ref" >/dev/null ||
        { add "capability '$id' cites $h doc '${ref%%#*}', which no snapshot in $h-mock/snapshots/MANIFEST.yaml copies"; continue; }
      doc_ref_section "$h" "$ref" >/dev/null ||
        add "capability '$id' cites '$ref', but the $h snapshot of that page has no such section"
    fi
  done < <(jq -r --arg h "$h" '.[] | .id as $id | (.doc.providers[$h] // false) | select(type == "object")
            | ((.runs // [])[] | [$id, ., "run"]), ((.docs // [])[] | [$id, ., "doc"]) | @tsv' <<<"$caps")
done
[ -z "$problems" ] && exit 0
refuse "Snapshots of the real harness are stale, broken or unused:
${problems}"
