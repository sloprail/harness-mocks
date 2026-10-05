#!/usr/bin/env bash
# Per harness with snapshots. The harness binary and the doc pages are two different freezes:
# a run records the binary it was captured with (run.yaml `version`), the MANIFEST freezes each
# doc page by its own sha256 and `pin` says which binary capture.sh runs next. Nothing ties a
# run's version, or a page, to `pin`.
#   script <harness>-mock/snapshots/capture.sh exists: the only writer
#   docs   every MANIFEST.docs entry's live page still hashes to its sha256 (the
#          text is cached, never committed)
#   runs   kebab name; run.yaml has a version; ≥1 sample; sample dirs are
#          UTC timestamps YYYYMMDD-HHMMSS; each has events.jsonl; no two samples
#          have identical events (a re-run that changed nothing adds nothing);
#          each sample's SEAL lists exactly its files, with matching sha256s;
#          each sample is clean and in the mock's mode (_lib/recording.sh): every line of
#          its side-channel log (payloads.jsonl) is JSON, the harness wrote no error line
#          to stderr nor an error frame to the stream, and run.yaml's command is the
#          non-interactive structured-output mode the mock imitates. A scenario that records
#          one on purpose declares it in expected.yaml
# Only the harness this check's subject names, when the rule is split (subjects.sh).
# Per capability cell: every cited doc URL is copied in MANIFEST.docs and its
# #anchor resolves to a heading in that copy; every cited run path exists. A run no capability cites fails.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/snapshots.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/recording.sh"
RECORDING_EXPECTED="${SR_GUARDRAIL_DIR:-.}/expected.yaml"
load_spec capabilities; caps="$SPEC"

problems=""
add() { case "$problems" in *"- $1"$'\n'*) ;; *) problems="${problems}- $1"$'\n' ;; esac; }
hash() { shasum -a 256 "$1" | cut -d' ' -f1; }

if [ -n "$(subject_id)" ]; then hs="$(subject_id)"; else hs="$(harnesses)"; fi
for h in $hs; do
  d="$(snap_dir "$h")"
  cited="$(jq -c --arg h "$h" '[.[] | .doc.providers[$h] // false | select(type == "object")]' <<<"$caps")"
  if [ ! -d "$d" ]; then
    [ "$(jq 'length' <<<"$cited")" -eq 0 ] || add "$h-mock/snapshots/ is missing, but capabilities cite snapshots for '$h'"
    continue
  fi
  m="$(yq -o=json '.' "$d/MANIFEST.yaml" 2>/dev/null)" || { add "$h-mock/snapshots/MANIFEST.yaml is missing or not valid YAML"; continue; }
  [ -n "$(jq -r '.pin // ""' <<<"$m")" ] || { add "$h-mock/snapshots/MANIFEST.yaml has no pin: capture.sh pin <version> sets the harness version captures run with"; continue; }
  [ -f "$d/capture.sh" ] || add "$h-mock/snapshots/capture.sh is missing: snapshots are only written by it"

  # docs: only their hashes are committed; the live page must still hash to it
  # (else the doc changed since it was frozen, and the snapshot is stale).
  for u in $(jq -r '(.docs // {}) | keys[]' <<<"$m"); do
    doc_copy "$h" "$u" >/dev/null || add "$DOC_ERROR"
  done

  for r in "$d"/runs/*/; do
    [ -d "$r" ] || continue
    name="$(basename "$r")"
    kebab "$name" || add "$h-mock/snapshots/runs/$name: the name must be kebab-case"
    rv="$(yq -r '.version // ""' "$r/run.yaml" 2>/dev/null)"
    [ -n "$rv" ] || add "run $h/$name has no version in its run.yaml: the harness binary it was captured with"
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
      while IFS= read -r p; do [ -n "$p" ] && add "$p"; done < <(recording_problems "$h" "$name" "$r" "$s")
      hv="$(hash "$s/events.jsonl")"
      dup="$(printf '%s\n' "$seen" | awk -v h="$hv" '$1 == h {print $2}')"
      [ -z "$dup" ] || add "run $h/$name: sample $ts has the same events as $dup; drop it"
      seen="$seen$hv $ts"$'\n'
    done
    [ "$n" -gt 0 ] || add "run $h/$name has no samples"
    jq -e --arg n "$h-mock/snapshots/runs/$name" 'any(.[]; (.runs // []) | index($n))' <<<"$cited" >/dev/null ||
      add "run $h/$name is cited by no capability: cite it, or delete it"
  done

  while IFS=$'\t' read -r id ref kind; do
    [ -n "$id" ] || continue
    if [ "$kind" = run ]; then
      [ -d "$SR_TREE/$ref" ] || add "capability '$id' cites run '$ref', which does not exist"
    else
      doc_sha "$h" "$ref" >/dev/null ||
        { add "capability '$id' cites $h doc '${ref%%#*}', which no snapshot in $h-mock/snapshots/MANIFEST.yaml freezes"; continue; }
      if ! doc_copy "$h" "$ref" >/dev/null; then add "capability '$id' cites '$ref': $DOC_ERROR"; continue; fi
      doc_ref_section "$h" "$ref" >/dev/null ||
        add "capability '$id' cites '$ref', but the frozen page has no such section"
    fi
  done < <(jq -r --arg h "$h" '.[] | .id as $id | (.doc.providers[$h] // false) | select(type == "object")
            | ((.runs // [])[] | [$id, ., "run"]), ((.docs // [])[] | [$id, ., "doc"]) | @tsv' <<<"$caps")
done
[ -z "$problems" ] && exit 0
refuse "Snapshots of the real harness are stale, broken or unused:
${problems}"
