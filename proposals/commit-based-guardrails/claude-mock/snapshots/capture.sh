#!/usr/bin/env bash
# The ONLY writer of claude-mock/snapshots/ (the snapshots-read-only gate
# refuses every other write). Everything it writes is sealed; snapshots-current
# refuses a snapshot whose seal does not match, so a hand edit is caught at the
# commit even if it slipped past the gate.
#
#   capture.sh run <name>    run the scenario in runs/<name>/setup/ against the
#                            real `claude`, and add a sample
#   capture.sh doc <url>     copy a doc page (e.g. https://code.claude.com/docs/en/hooks)
#   capture.sh all           re-capture every run and doc at the installed
#                            claude's version, and set MANIFEST.version to it
#
# A scenario is hand-authored, and only its setup/ is:
#   runs/<name>/setup/prompt.txt       the prompt
#   runs/<name>/setup/settings.json    the project's .claude/settings.json (hooks)
#   runs/<name>/setup/hook.sh          optional: the hook every event runs;
#                                      it appends its stdin to $HOOK_LOG
#   runs/<name>/setup/args             optional: extra claude flags, one per line
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
manifest="$here/MANIFEST.yaml"
die() { echo "capture.sh: $*" >&2; exit 1; }
[ -f "$manifest" ] || die "no MANIFEST.yaml beside this script"
version() { yq -r '.version' "$manifest"; }
installed() { claude --version | awk '{print $1}'; }

# seal DIR — SEAL lists the sha256 of every other file under DIR.
seal() { (cd "$1" && find . -type f ! -name SEAL | LC_ALL=C sort | xargs shasum -a 256 >SEAL); }

# normalize — hook payloads and stream frames into events.jsonl: what a
# scenario did, without what differs between two captures of the same
# behaviour (ids, paths, timings, the model's own wording).
normalize() {
  local cap="$1"
  jq -c 'walk(if type == "object" then del(.session_id, .transcript_path, .cwd, .agent_id, .tool_use_id,
          .uuid, .parentUuid, .timestamp, .duration_ms, .durationMs, .last_assistant_message) else . end)
         | {event: "hook", hook: .hook_event_name, payload: .}' "$cap/payloads.jsonl" 2>/dev/null || true
  jq -c 'select(.type != "assistant" and .type != "user")
         | {event: "stream", type, subtype: (.subtype // null)}' "$cap/stream.jsonl" 2>/dev/null || true
}

capture_run() {
  local name="$1" run="$here/runs/$1" v ts work home cap
  [ -f "$run/setup/prompt.txt" ] || die "runs/$name/setup/prompt.txt is missing: author the scenario first"
  v="$(version)"; [ "$(installed)" = "$v" ] || die "installed claude is $(installed), MANIFEST.version is $v: run 'capture.sh all' to move to it"
  ts="$(date -u +%Y%m%d-%H%M%S)"
  work="$(mktemp -d)"; home="$work/home"; cap="$run/samples/$ts"
  mkdir -p "$work/repo/.claude" "$home/Library" "$cap"
  ln -s "$HOME/Library/Keychains" "$home/Library/Keychains" 2>/dev/null || true   # keeps the login, nothing else
  cp "$run/setup/settings.json" "$work/repo/.claude/settings.json" 2>/dev/null || true
  [ -f "$run/setup/hook.sh" ] && cp "$run/setup/hook.sh" "$work/repo/hook.sh" && chmod +x "$work/repo/hook.sh"
  git -C "$work/repo" init -q && git -C "$work/repo" -c commit.gpgsign=false commit -q --allow-empty -m init
  args=(); [ -f "$run/setup/args" ] && while IFS= read -r a; do [ -n "$a" ] && args+=("$a"); done <"$run/setup/args"
  set +e
  (cd "$work/repo" && HOME="$home" HOOK_LOG="$cap/payloads.jsonl" \
    claude -p --model haiku --dangerously-skip-permissions --output-format stream-json --verbose \
      ${args[@]+"${args[@]}"} "$(cat "$run/setup/prompt.txt")" >"$cap/stream.jsonl" 2>"$cap/stderr.txt")
  echo $? >"$cap/exit.txt"
  set -e
  mkdir -p "$cap/transcript"
  cp -R "$home/.claude/projects/"*/* "$cap/transcript/" 2>/dev/null || true
  # sanitize: the machine's paths out of everything captured
  grep -rlF -e "$work" -e "$HOME" "$cap" 2>/dev/null | while IFS= read -r f; do
    sed -i '' -e "s#$work/repo#<RUN>#g" -e "s#$work#<TMP>#g" -e "s#$HOME#<HOME>#g" "$f"
  done
  normalize "$cap" >"$cap/events.jsonl"
  rm -rf "$work"
  # a re-capture with the same events adds nothing
  for other in "$run"/samples/*/; do
    [ "$other" = "$cap/" ] && continue
    if cmp -s "$other/events.jsonl" "$cap/events.jsonl"; then
      rm -rf "$cap"; echo "same events as $(basename "$other"): no new sample"; return 0
    fi
  done
  printf 'version: %s\ncommand: claude -p --model haiku --dangerously-skip-permissions --output-format stream-json --verbose\n' "$v" >"$run/run.yaml"
  seal "$cap"
  echo "captured runs/$name/samples/$ts"
}

capture_doc() {
  local url="${1%%#*}" page v tmp
  page="$(basename "$url").md"
  v="$(version)"; tmp="$(mktemp)"
  curl -fsSL "$url.md" -o "$tmp" || die "could not fetch $url.md"
  head -c 200 "$tmp" | grep -q '<html' && die "$url.md is not markdown"
  mkdir -p "$here/docs"; mv "$tmp" "$here/docs/$page"; chmod 644 "$here/docs/$page"
  yq -i ".docs[\"$page\"] = {\"url\": \"$url\", \"version\": \"$v\", \"sha256\": \"$(shasum -a 256 "$here/docs/$page" | cut -d' ' -f1)\"}" "$manifest"
  echo "copied $url → docs/$page"
}

case "${1:-}" in
  run) [ -n "${2:-}" ] || die "usage: capture.sh run <name>"; capture_run "$2" ;;
  doc) [ -n "${2:-}" ] || die "usage: capture.sh doc <url>"; capture_doc "$2" ;;
  all)
    v="$(installed)"; yq -i ".version = \"$v\"" "$manifest"
    for r in "$here"/runs/*/; do rm -rf "$r/samples"; capture_run "$(basename "$r")"; done
    for u in $(yq -r '.docs // {} | to_entries[] | .value.url' "$manifest"); do capture_doc "$u"; done
    ;;
  *) die "usage: capture.sh run <name> | doc <url> | all" ;;
esac
