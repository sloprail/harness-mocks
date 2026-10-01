#!/usr/bin/env bash
# The ONLY writer of claude-mock/snapshots/ (the snapshots-read-only gate
# refuses every other write). Everything it writes is sealed; snapshots-current
# refuses a snapshot whose seal does not match, so a hand edit is caught at the
# commit even if it slipped past the gate.
#
#   capture.sh run <name>    run the scenario in runs/<name>/setup/ against the
#                            real `claude`, and add a sample
#   capture.sh doc <url>     freeze a doc page (e.g. https://code.claude.com/docs/en/hooks):
#                            its sha256 goes in the MANIFEST; its text only into a cache
#                            under the git dir, never into the repo
#   capture.sh drop <run> <ts>  remove one sample (a bad or non-hermetic capture)
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
version() { yq -r '.version' "$manifest"; }
installed() { claude --version | awk '{print $1}'; }
# The first capture creates the MANIFEST, frozen at the installed version: it is
# a snapshot file too, so nothing else may write it.
[ -f "$manifest" ] || printf 'version: "%s"\n' "$(installed)" >"$manifest"

# seal DIR — SEAL lists the sha256 of every other file under DIR.
# Written to SEAL.tmp and renamed only on success, so a SEAL always means a
# complete sample (the cleanup trap relies on it).
seal() { (cd "$1" && find . -type f ! -name 'SEAL*' | LC_ALL=C sort | xargs shasum -a 256 >SEAL.tmp && mv SEAL.tmp SEAL); }

# normalize — hook payloads and stream frames into events.jsonl: what a
# scenario did, without what differs between two captures of the same
# behaviour (ids, paths, timings, the model's own wording).
# The run's own session id is kept as <SESSION_ID> wherever it appears (a child's
# env, a command's output): which session an id names is behaviour, its value is not.
normalize() {
  local cap="$1" sid
  sid="$(jq -r 'select(.session_id) | .session_id' "$cap/payloads.jsonl" 2>/dev/null | head -n1)"
  jq -c --arg sid "${sid:-<none>}" 'walk(if type == "object" then del(.session_id, .transcript_path, .cwd, .agent_id,
          .tool_use_id, .prompt_id, .uuid, .parentUuid, .timestamp, .duration_ms, .durationMs, .last_assistant_message)
          elif type == "string" then gsub($sid; "<SESSION_ID>") else . end)
         | del(.tool_input.description?) | {event: "hook", hook: .hook_event_name, payload: .}' "$cap/payloads.jsonl" 2>/dev/null || true
  jq -c 'select(.type != "assistant" and .type != "user")
         | {event: "stream", type, subtype: (.subtype // null)}' "$cap/stream.jsonl" 2>/dev/null || true
}

capture_run() {
  local name="$1" run="$here/runs/$1" v ts home
  work="" cap=""   # global: the EXIT trap below reads them
  [ -f "$run/setup/prompt.txt" ] || die "runs/$name/setup/prompt.txt is missing: author the scenario first"
  v="$(version)"; [ "$(installed)" = "$v" ] || die "installed claude is $(installed), MANIFEST.version is $v: run 'capture.sh all' to move to it"
  ts="$(date -u +%Y%m%d-%H%M%S)"
  cap="$run/samples/$ts"
  [ ! -e "$cap" ] || die "a sample named $ts already exists; re-run in a second"
  work="$(mktemp -d)"; home="$work/home"
  # a capture that fails part-way leaves nothing behind: no half-written sample
  trap 'rm -rf "$work"; [ -f "$cap/SEAL" ] || rm -rf "$cap"' EXIT
  mkdir -p "$work/repo/.claude" "$home/Library" "$cap"
  mkdir -m 700 "$work/tmp"   # private: claude refuses a shared temp root for its per-uid dir
  ln -s "$HOME/Library/Keychains" "$home/Library/Keychains" 2>/dev/null || true   # keeps the login, nothing else
  cp "$run/setup/settings.json" "$work/repo/.claude/settings.json" 2>/dev/null || true
  [ -f "$run/setup/hook.sh" ] && cp "$run/setup/hook.sh" "$work/repo/hook.sh" && chmod +x "$work/repo/hook.sh"
  git -C "$work/repo" init -q && git -C "$work/repo" -c commit.gpgsign=false commit -q --allow-empty -m init
  args=(); [ -f "$run/setup/args" ] && while IFS= read -r a; do [ -n "$a" ] && args+=("$a"); done <"$run/setup/args"
  # Hermetic: claude starts from an EMPTY environment plus only what it needs to
  # run, so nothing of the session that runs this script (CLAUDECODE,
  # CLAUDE_CODE_SESSION_ID, CLAUDE_CODE_ENTRYPOINT, …) can leak into what the
  # capture records as the harness's own behaviour. stdin is closed so a -p run
  # does not wait for it. CLAUDE_CODE_TMPDIR too: on macOS claude ignores TMPDIR
  # for its own per-uid dir (/tmp/claude-<uid>), which the session running this
  # script shares; left there, a capture could see that session's scratchpad.
  # (A literal /tmp path stays shared: only a sandbox could stop that.)
  local claude_bin; claude_bin="$(command -v claude)" || die "claude is not on PATH"
  set +e
  (cd "$work/repo" && env -i PATH="$PATH" HOME="$home" USER="${USER:-}" LANG="${LANG:-en_US.UTF-8}" \
    TERM="${TERM:-dumb}" TMPDIR="$work/tmp" CLAUDE_CODE_TMPDIR="$work/tmp" HOOK_LOG="$cap/payloads.jsonl" \
    "$claude_bin" -p --model haiku --dangerously-skip-permissions --output-format stream-json --verbose \
      ${args[@]+"${args[@]}"} "$(cat "$run/setup/prompt.txt")" </dev/null >"$cap/stream.jsonl" 2>"$cap/stderr.txt")
  echo $? >"$cap/exit.txt"
  set -e
  mkdir -p "$cap/transcript"
  cp -R "$home/.claude/projects/"*/* "$cap/transcript/" 2>/dev/null || true
  # sanitize: the machine's paths out of everything captured
  { grep -rlF -e "$work" -e "$HOME" "$cap" 2>/dev/null || true; } | while IFS= read -r f; do
    sed -i '' -e "s#$work/repo#<RUN>#g" -e "s#$work#<TMP>#g" -e "s#$HOME#<HOME>#g" "$f"
  done
  normalize "$cap" >"$cap/events.jsonl"
  # a re-capture with the same events adds nothing
  for other in "$run"/samples/*/; do
    [ "$other" = "$cap/" ] && continue
    if cmp -s "$other/events.jsonl" "$cap/events.jsonl"; then
      trap - EXIT; rm -rf "$cap" "$work"; echo "same events as $(basename "$other"): no new sample"; return 0
    fi
  done
  printf 'version: %s\ncommand: claude -p --model haiku --dangerously-skip-permissions --output-format stream-json --verbose\n' "$v" >"$run/run.yaml"
  seal "$cap"
  trap - EXIT; rm -rf "$work"
  echo "captured runs/$name/samples/$ts"
}

# capture_doc URL — freeze a doc page: fetch <url>.md, record its sha256 in the
# MANIFEST, and keep the text only in the cache under the git dir (the rules'
# doc_copy reads it there). The page's text is never committed: it is the
# harness vendor's.
capture_doc() {
  local url="${1%%#*}" v tmp sha cache
  url="${url%/}"; v="$(version)"; tmp="$(mktemp)"
  curl -fsSL "$url.md" -o "$tmp" || die "could not fetch $url.md"
  head -c 200 "$tmp" | grep -q '<html' && die "$url.md is not markdown"
  sha="$(shasum -a 256 "$tmp" | cut -d' ' -f1)"
  cache="$(git -C "$here" rev-parse --path-format=absolute --git-common-dir)/sloprail-doc-cache"
  mkdir -p "$cache" && mv "$tmp" "$cache/$sha.md"
  URL="$url" V="$v" SHA="$sha" yq -i '.docs[strenv(URL)] = {"version": strenv(V), "sha256": strenv(SHA)}' "$manifest"
  echo "froze $url at sha256 $sha (text cached, not committed)"
}

case "${1:-}" in
  run) [ -n "${2:-}" ] || die "usage: capture.sh run <name>"; capture_run "$2" ;;
  drop)
    # The one way to remove a sample: this script is the only writer of
    # snapshots/, and a hand `rm` there is refused by gate/snapshots-read-only.
    [ -n "${2:-}" ] && [ -n "${3:-}" ] || die "usage: capture.sh drop <run> <sample-timestamp>"
    printf '%s' "$3" | grep -Eq '^[0-9]{8}-[0-9]{6}$' || die "a sample is named YYYYMMDD-HHMMSS"
    [ -d "$here/runs/$2/samples/$3" ] || die "no sample runs/$2/samples/$3"
    rm -rf "$here/runs/$2/samples/$3"
    echo "dropped runs/$2/samples/$3"
    ;;
  doc) [ -n "${2:-}" ] || die "usage: capture.sh doc <url>"; capture_doc "$2" ;;
  all)
    v="$(installed)"; yq -i ".version = \"$v\"" "$manifest"
    for r in "$here"/runs/*/; do rm -rf "$r/samples"; capture_run "$(basename "$r")"; done
    for u in $(yq -r '.docs // {} | keys | .[]' "$manifest"); do capture_doc "$u"; done
    ;;
  *) die "usage: capture.sh run <name> | drop <run> <ts> | doc <url> | all" ;;
esac
