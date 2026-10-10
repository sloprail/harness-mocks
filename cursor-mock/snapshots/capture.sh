#!/usr/bin/env bash
# The ONLY writer of cursor-mock/snapshots/ (the snapshots-read-only gate
# refuses every other write). Everything it writes is sealed; snapshots-current
# refuses a snapshot whose seal does not match, so a hand edit is caught at the
# commit even if it slipped past the gate.
#
#   capture.sh run <name> [--rerecord]
#                            run the scenario in runs/<name>/setup/ against the
#                            pinned `cursor-agent`, and add a sample. A run recorded at
#                            another version than the pin is refused (its samples
#                            would mix versions) unless --rerecord drops them first
#   capture.sh pin <version> install that exact cursor-agent (tools/harness-bin) and make it
#                            the one captures run; nothing already recorded changes
#   capture.sh drop <run> <ts>  remove one sample (a bad or non-hermetic capture)
#   capture.sh all           re-record every run at the pin (each re-freezes its docs, below)
#
# Docs follow recordings. Recording a run (run, all) also re-freezes the doc pages this
# harness's capability cells cite (spec/capabilities/*.yaml, providers.cursor.docs) that are not
# already frozen at the live page's hash: the MANIFEST holds each page's sha256 and fetch date, the
# text only a cache under the git dir, never the repo. There is no standalone doc re-freeze: an
# upstream doc that changed is no PR's problem (no check reads the live website); the
# next recording that cites it pulls it.
#
# Two versions, kept apart: the cursor-agent a run was recorded with is the run's own (run.yaml),
# and a doc page is frozen by its own sha256 (MANIFEST docs). MANIFEST `pin` is only which
# cursor-agent this script runs. It is never the global one on PATH: tools/harness-bin installs the
# pin into a cache of its own and refuses a binary that does not report exactly it.
# cursor-agent updates itself in the background unless run with --disable-auto-update
# (a hidden flag); the capture passes it. The command a run.yaml records leaves it out: it is
# housekeeping, not behaviour a scenario shows.
#
# cursor-agent has a versioned download (tools/harness-bin knows the build hash of each date
# it has been pinned at), but the official installer only ever installs the latest one.
#
# The version is cursor-agent's release date (`2026.09.28`): `--version` also
# prints a build hash after it (`2026.09.28-64d2043`), which the schema's
# version pattern does not take, so the hash is dropped.
#
# Login: cursor-agent has no environment-only login in this setup, so the fake
# HOME links to the user's Keychains directory and nothing else (the same as
# claude-mock's capture). No credential is copied into the fake HOME; the
# email the login reports is nulled out of every payload before anything is
# sealed. A CURSOR_API_KEY is never read or passed.
#
# A scenario is hand-authored, and only its setup/ is:
#   runs/<name>/setup/prompt.txt       the prompt
#   runs/<name>/setup/hooks.json       the project's .cursor/hooks.json
#   runs/<name>/setup/user-hooks.json  optional: the user's ~/.cursor/hooks.json (a second hook source)
#   runs/<name>/setup/*.sh             optional: the hook scripts, installed in .cursor/hooks/;
#                                      hook.sh appends to $HOOK_LOG
#   runs/<name>/setup/args             optional: extra cursor-agent flags, one per line
#   runs/<name>/setup/no-force         optional: an empty file; the run leaves --force off
#   runs/<name>/setup/prepare.sh       optional: run in the scratch repo before cursor-agent, with
#                                      HOME the run's (e.g. to lay out a plugin directory)
#   runs/<name>/setup/then-<NN>-prompt.txt   optional later steps, run in name order under the
#                                      same HOME, with then-<NN>-args (a line "<SESSION>" is the
#                                      first step's session id) and then-<NN>-cwd (a directory
#                                      name, next to the repo, to run from)
#   runs/<name>/setup/env              optional: KEY=VALUE lines cursor-agent inherits on
#                                      top of the hermetic env
#   runs/<name>/setup/tui.yaml         optional: makes it an interactive run. The scenario is
#                                      played against cursor-agent's TUI, not run with -p, by
#                                      tools/tui-record from this script (what to wait for on the
#                                      screen or in the hook log, what to type; see that tool).
#                                      prompt.txt is the text it types (text_file: prompt.txt).
#                                      Such a run has no stream.jsonl; tui.jsonl is the steps done
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
manifest="$here/MANIFEST.yaml"
die() { echo "capture.sh: $*" >&2; exit 1; }
root="$(cd "$here/../.." && pwd)"
pin() { yq -r '.pin // ""' "$manifest" 2>/dev/null; }
hbin() { (cd "$root" && go run ./tools/harness-bin "$@"); }
# pinned_bin — the cursor-agent binary of exactly the pin, from tools/harness-bin's cache. It dies
# unless that install exists and reports the pin (a build of it, for a bare cursor date): never the cursor-agent on PATH.
pinned_bin() {
  local v bin; v="$(pin)"; [ -n "$v" ] || die "MANIFEST.yaml has no pin: run 'capture.sh pin <version>'"
  # harness-bin path is the verification: it fails unless the install exists AND reports the pin
  # (it says which, on stderr), so the version is not compared a second time here.
  bin="$(hbin path cursor "$v")" || die "cursor-agent $v cannot be used for captures (the reason is harness-bin's, above): run 'capture.sh pin $v'"
  printf '%s' "$bin"
}

# seal DIR — SEAL lists the sha256 of every other file under DIR.
# Written to SEAL.tmp and renamed only on success, so a SEAL always means a
# complete sample (the cleanup trap relies on it).
seal() { (cd "$1" && find . -type f ! -name 'SEAL*' | LC_ALL=C sort | xargs shasum -a 256 >SEAL.tmp && mv SEAL.tmp SEAL); }

# normalize — hook payloads and stream frames into events.jsonl: what a
# scenario did, without what differs between two captures of the same
# behaviour (ids, paths, timings, the model's own wording, how many thoughts
# it voiced; how full the context was and how many messages it held when the user asked for a compaction: 4 in one capture, 6 in the next). The run's own session id is kept as <SESSION_ID> wherever it
# appears, so that they match is recorded; which session a value names is
# behaviour, the id itself is not.
normalize() {
  local cap="$1" sid
  sid="$(jq -r 'select(.session_id) | .session_id' "$cap/payloads.jsonl" 2>/dev/null | head -n1 || true)"
  jq -c --arg sid "${sid:-<none>}" 'select(.hook_event_name != "afterAgentThought")
         | (if .trigger == "manual" then del(.context_tokens, .context_usage_percent, .message_count, .messages_to_compact) else . end)
         | walk(if type == "object" then del(.transcript_path, .cwd, .workspace_roots, .user_email, .generation_id, .tool_use_id,
              .duration, .duration_ms, .model, .model_id, .model_params, .cursor_version, .conversation_id,
              .input_tokens, .output_tokens, .cache_read_tokens, .cache_write_tokens)
          elif type == "string" then gsub($sid; "<SESSION_ID>") else . end)
         | {event: "hook", hook: .hook_event_name, payload: .}' "$cap/payloads.jsonl" 2>/dev/null || true
  jq -c 'select(.type != "assistant" and .type != "user" and .type != "thinking")
         | if .type == "tool_call" then (.tool_call | to_entries | map(select(.key | endswith("ToolCall")))[0]) as $t
             | {event: "stream", type, subtype, tool: $t.key,
                outcome: (if .subtype == "completed" then ($t.value.result | keys | map(select(. != "isBackground"))[0]) else null end)}
           else {event: "stream", type, subtype: (.subtype // null)} end' "$cap/stream.jsonl" 2>/dev/null || true
}

# one capture at a time: the model calls are the user's account's
lock=/tmp/capture-cursor.lock.d
acquire() { local n=0; until mkdir "$lock" 2>/dev/null; do n=$((n + 1)); [ "$n" -lt 600 ] || die "another cursor capture holds $lock"; sleep 1; done; }

capture_run() {
  local name="$1" rerecord="${2:-}" run="$here/runs/$1" v ts home rv bin
  work="" cap="" lname=""   # global: the EXIT trap below reads them
  [ -f "$run/setup/prompt.txt" ] || die "runs/$name/setup/prompt.txt is missing: author the scenario first"
  v="$(pin)"; bin="$(pinned_bin)" || exit 1
  rv="$(yq -r '.version // ""' "$run/run.yaml" 2>/dev/null || true)"
  if [ -n "$rv" ] && [ "$rv" != "$v" ] && ls "$run"/samples/*/ >/dev/null 2>&1; then
    [ "$rerecord" = --rerecord ] || die "runs/$name was recorded at cursor-agent $rv, the pin is $v: a sample added now would mix versions; re-record it with 'capture.sh run $name --rerecord'"
    rm -rf "$run/samples"
  fi
  acquire
  ts="$(date -u +%Y%m%d-%H%M%S)"
  cap="$run/samples/$ts"
  # a capture that fails part-way leaves nothing behind: no half-written sample
  trap 'rm -rf "$work"; [ -f "$cap/SEAL" ] || rm -rf "$cap"; rmdir "$lock" 2>/dev/null || true' EXIT
  [ ! -e "$cap" ] || die "a sample named $ts already exists; re-run in a second"
  work="$(cd "$(mktemp -d)" && pwd -P)"; home="$work/home"   # canonical (/private/var/…), so sanitizing matches the paths cursor-agent records
  mkdir -p "$work/repo/.cursor/hooks" "$home/Library" "$cap"
  mkdir -m 700 "$work/tmp"
  ln -s "$HOME/Library/Keychains" "$home/Library/Keychains" 2>/dev/null || true   # keeps the login, nothing else
  cp "$run/setup/hooks.json" "$work/repo/.cursor/hooks.json" 2>/dev/null || true
  # optional user source: setup/user-hooks.json is the user's ~/.cursor/hooks.json
  if [ -f "$run/setup/user-hooks.json" ]; then mkdir -p "$home/.cursor"; cp "$run/setup/user-hooks.json" "$home/.cursor/hooks.json"; fi
  for s in "$run"/setup/*.sh; do [ -f "$s" ] && [ "$(basename "$s")" != prepare.sh ] && cp "$s" "$work/repo/.cursor/hooks/" && chmod +x "$work/repo/.cursor/hooks/$(basename "$s")"; done
  git -C "$work/repo" init -q && git -C "$work/repo" -c commit.gpgsign=false commit -q --allow-empty -m init
  [ ! -f "$run/setup/prepare.sh" ] || (cd "$work/repo" && env HOME="$home" TMPDIR="$work/tmp" sh "$run/setup/prepare.sh")
  args=(); [ -f "$run/setup/args" ] && while IFS= read -r a; do [ -n "$a" ] && args+=("$a"); done <"$run/setup/args"
  extra=(); [ -f "$run/setup/env" ] && while IFS= read -r a; do [ -n "$a" ] && extra+=("$a"); done <"$run/setup/env"
  # Hermetic: cursor-agent starts from an EMPTY environment plus only what it needs
  # to run, so nothing of the session that runs this script can leak into what
  # the capture records as the harness's own behaviour. stdin is closed so a -p
  # run does not wait for it. (A literal /tmp path stays shared: only a sandbox
  # could stop that.)
  set +e
  # A scenario's `symlink` file names a symlink to the repo ("<name>"): the run
  # starts from it, so cursor-agent's working directory is a symlinked path.
  cwd="$work/repo"
  if [ -f "$run/setup/symlink" ]; then read -r lname <"$run/setup/symlink"; ln -s "$work/repo" "$work/$lname"; cwd="$work/$lname"; fi
  # A scenario's `no-force` file leaves --force off: print mode as the headless doc
  # describes it without the flag (the run's command is recorded as it ran).
  force="--force"; [ -f "$run/setup/no-force" ] && force=""
  # One cursor-agent invocation per step: the scenario's own prompt.txt and args
  # are step 1, and each setup/then-<NN>-prompt.txt (with then-<NN>-args, then-<NN>-cwd)
  # is a later one, in name order, under the same fake HOME, so a later step can
  # resume what an earlier one left. A step's args may name the first step's
  # session as <SESSION> (it is not known before the run), and its cwd file names
  # a directory next to the repo to run it from (created, with the repo's .cursor
  # hooks). exit.txt has one line per step.
  : >"$cap/stream.jsonl"; : >"$cap/stderr.txt"; : >"$cap/exit.txt"
  steps=(""); for f in "$run"/setup/then-*-prompt.txt; do [ -f "$f" ] && steps+=("$(basename "$f" prompt.txt)"); done
  # An interactive run: a scenario with a setup/tui.yaml is played against cursor-agent's TUI by
  # tools/tui-record (one recorder for every harness's TUI), not run with -p. The TUI has no
  # stream: the hook log (payloads.jsonl) and the transcript are what it leaves, and tui.jsonl
  # the steps the script did. The hook scripts, HOME, TMPDIR and HOOK_LOG are the same as below.
  if [ -f "$run/setup/tui.yaml" ]; then
    rm -f "$cap/stream.jsonl" "$cap/stderr.txt"; steps=()
    targs=(); for a in --disable-auto-update --trust --model auto ${args[@]+"${args[@]}"}; do targs+=(--arg "$a"); done
    for a in ${extra[@]+"${extra[@]}"}; do targs+=(--env "$a"); done
    (cd "$root" && go run ./tools/tui-record --bin "$bin" --expect-version "$v" --script "$run/setup/tui.yaml" \
      --cwd "$cwd" --home "$home" --tmp "$work/tmp" --hook-log "$cap/payloads.jsonl" \
      --log "$cap/tui.jsonl" --exit-file "$cap/exit.txt" "${targs[@]}") || die "tools/tui-record failed: no sample is kept"
  fi
  for step in ${steps[@]+"${steps[@]}"}; do
    sargs=(); if [ -z "$step" ]; then sargs=(${args[@]+"${args[@]}"}); elif [ -f "$run/setup/${step}args" ]; then
      while IFS= read -r a; do
        [ -n "$a" ] || continue
        [ "$a" != "<SESSION>" ] || a="$(jq -r 'select(.session_id) | .session_id' "$cap/stream.jsonl" | head -n1)"
        sargs+=("$a")
      done <"$run/setup/${step}args"
    fi
    sdir="$cwd"
    if [ -f "$run/setup/${step}cwd" ]; then sdir="$work/$(cat "$run/setup/${step}cwd")"; mkdir -p "$sdir"; cp -R "$work/repo/.cursor" "$sdir/"; fi
    (cd "$sdir" && env -i PATH="$PATH" HOME="$home" USER="${USER:-}" LANG="${LANG:-en_US.UTF-8}" \
      TERM="${TERM:-dumb}" TMPDIR="$work/tmp" HOOK_LOG="$cap/payloads.jsonl" ${extra[@]+"${extra[@]}"} \
      "$bin" --disable-auto-update -p ${force:+"$force"} --trust --model auto --output-format stream-json \
        ${sargs[@]+"${sargs[@]}"} "$(cat "$run/setup/${step}prompt.txt")" </dev/null >>"$cap/stream.jsonl" 2>>"$cap/stderr.txt")
    echo $? >>"$cap/exit.txt"
  done
  set -e
  mkdir -p "$cap/transcript"
  # a conversation resumed from another directory has a transcript there too: with
  # several projects, each is kept apart under the name of its directory
  local enc_work; enc_work="$(printf '%s' "${work#/}" | sed 's#[^A-Za-z0-9]#-#g')"
  local projs=("$home/.cursor/projects/"*/agent-transcripts)
  if [ "${#projs[@]}" -gt 1 ]; then
    local p; for p in "${projs[@]}"; do
      mkdir -p "$cap/transcript/$(basename "$(dirname "$p")" | sed "s#^${enc_work}-##")"; cp -R "$p/"* "$cap/transcript/$(basename "$(dirname "$p")" | sed "s#^${enc_work}-##")/"
    done
  else
    cp -R "$home/.cursor/projects/"*/agent-transcripts/* "$cap/transcript/" 2>/dev/null || true
  fi
  # the transcript stamps the wall clock and the user's timezone into the prompt
  find "$cap/transcript" -name '*.jsonl' -type f | while IFS= read -r f; do
    jq -c 'walk(if type == "string" then gsub("<timestamp>[^<]*</timestamp>"; "<timestamp/>") else . end)' "$f" >"$f.tmp" && mv "$f.tmp" "$f"
  done
  # the login's email: every payload carries it as user_email; nulled, never kept
  if [ -f "$cap/payloads.jsonl" ]; then
    jq -c 'if type == "object" and has("user_email") then .user_email = null else . end' "$cap/payloads.jsonl" >"$cap/payloads.tmp" && mv "$cap/payloads.tmp" "$cap/payloads.jsonl"
  fi
  # redact: the value of every secret-named variable a child saw out of
  # everything captured, before anything is sealed or committed
  { jq -r 'select(.hook_env) | .hook_env | to_entries[] | select(.key | test("TOKEN|SECRET|KEY|PASSWORD")) | .value' "$cap/payloads.jsonl" 2>/dev/null || true
    grep -rhoE '[A-Z0-9_]*(TOKEN|SECRET|KEY|PASSWORD)[A-Z0-9_]*=[A-Za-z0-9._/+-]{8,}' "$cap" 2>/dev/null | cut -d= -f2- || true   # no secret is fine
  } | sort -u | while IFS= read -r secret; do
    [ "${#secret}" -ge 8 ] || continue
    { grep -rlF -e "$secret" "$cap" 2>/dev/null || true; } | while IFS= read -r f; do sed -i '' -e "s#$secret#<REDACTED>#g" "$f"; done
  done
  # sanitize: the machine's paths out of everything captured, and the run dir as
  # cursor-agent encodes it into a folder name (every non-alphanumeric as -)
  local enc; enc="$(printf '%s' "${work#/}/repo" | sed 's#[^A-Za-z0-9]#-#g')"
  local lenc="${enc%-repo}-${lname:-link}"   # the symlink's own encoded name (a scenario's `symlink` file)
  { grep -rlF -e "$work" -e "$HOME" -e "$enc" -e "$lenc" "$cap" 2>/dev/null || true; } | while IFS= read -r f; do
    sed -i '' -e "s#$work/repo#<RUN>#g" -e "s#$lenc#<LINK_DIRNAME>#g" -e "s#$work#<TMP>#g" -e "s#$HOME#<HOME>#g" -e "s#$enc#<RUN_DIRNAME>#g" "$f"
  done
  normalize "$cap" >"$cap/events.jsonl"
  # a sample never carries an email address but Anthropic's attribution one
  if grep -rhoE '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[a-z]{2,}' "$cap" | grep -vxq 'noreply@anthropic.com'; then
    grep -rlE '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[a-z]{2,}' "$cap" | sed "s#^$cap/#  #" >&2
    die "the capture holds an email address in the files above; not sealed, and removed"
  fi
  # nor the account's user name
  if [ "${#USER}" -ge 4 ] && grep -rqF -e "$USER" "$cap"; then
    grep -rlF -e "$USER" "$cap" | sed "s#^$cap/#  #" >&2
    die "the capture holds the user name in the files above; not sealed, and removed"
  fi
  # a re-capture with the same events adds nothing
  for other in "$run"/samples/*/; do
    [ "$other" = "$cap/" ] && continue
    if cmp -s "$other/events.jsonl" "$cap/events.jsonl"; then
      rm -rf "$cap" "$work"; echo "same events as $(basename "$other"): no new sample"; return 0
    fi
  done
  if [ -f "$run/setup/tui.yaml" ]; then
    printf 'version: %s\ncommand: cursor-agent --trust --model auto\n' "$v" >"$run/run.yaml"   # the TUI: no -p
  else
    printf 'version: %s\ncommand: cursor-agent -p %s--trust --model auto --output-format stream-json\n' "$v" "${force:+$force }" >"$run/run.yaml"
  fi
  seal "$cap"
  rm -rf "$work"
  echo "captured runs/$name/samples/$ts"
}

# capture_doc URL — freeze a doc page: fetch <url>.md, record its sha256 in the
# MANIFEST, and keep the text only in the cache under the git dir (the rules'
# doc_copy reads it there). The page's text is never committed: it is the
# harness vendor's. A page already frozen at the live hash is left as it is (its fetch date too).
capture_doc() {
  local url="${1%%#*}" tmp sha cache old
  url="${url%/}"; tmp="$(mktemp)"
  curl -fsSL "$url.md" -o "$tmp" || die "could not fetch $url.md"
  head -c 200 "$tmp" | grep -q '<html' && die "$url.md is not markdown"
  sha="$(shasum -a 256 "$tmp" | cut -d' ' -f1)"
  cache="$(git -C "$here" rev-parse --path-format=absolute --git-common-dir)/sloprail-doc-cache"
  old="$(U="$url" yq -r '.docs[strenv(U)].sha256 // ""' "$manifest" 2>/dev/null || true)"
  if [ "$old" = "$sha" ]; then mkdir -p "$cache" && mv "$tmp" "$cache/$sha.md"; echo "$url already frozen at sha256 $sha"; return 0; fi
  mkdir -p "$cache" && mv "$tmp" "$cache/$sha.md"
  URL="$url" FETCHED="$(date -u +%F)" SHA="$sha" yq -i '.docs[strenv(URL)] = {"sha256": strenv(SHA), "fetched": strenv(FETCHED)}' "$manifest"
  echo "froze $url at sha256 $sha (text cached, not committed)"
}

# cited_docs — the doc pages (no anchor) this harness's capability cells cite, one per line.
cited_docs() {
  local h; h="$(basename "$(dirname "$here")")"; h="${h%%-mock}"
  for f in "$root"/spec/capabilities/*.yaml; do
    [ -f "$f" ] || continue
    H="$h" yq -r '(.providers[strenv(H)] | select(tag == "!!map") | .docs // [])[]' "$f" 2>/dev/null || true
  done | sed 's/#.*$//' | sort -u
}

# refreeze_cited — a recording is when docs are pulled: re-freeze every cited page not already
# frozen at its live hash. Runs after every `run` and `all`, whether or not it added a sample.
refreeze_cited() {
  local u
  for u in $(cited_docs); do capture_doc "$u"; done
}

case "${1:-}" in
  run) [ -n "${2:-}" ] || die "usage: capture.sh run <name> [--rerecord]"; capture_run "$2" "${3:-}"; refreeze_cited ;;
  pin)
    [ -n "${2:-}" ] || die "usage: capture.sh pin <version>"
    hbin install cursor "$2" >/dev/null || die "could not install cursor-agent $2"
    if [ -f "$manifest" ]; then V="$2" yq -i '.pin = strenv(V)' "$manifest" || die "could not set the pin in $manifest"; else printf 'pin: "%s"\n' "$2" >"$manifest"; fi
    echo "pinned cursor-agent $2"
    ;;
  drop)
    # The one way to remove a sample: this script is the only writer of
    # snapshots/, and a hand `rm` there is refused by gate/snapshots-read-only.
    [ -n "${2:-}" ] && [ -n "${3:-}" ] || die "usage: capture.sh drop <run> <sample-timestamp>"
    printf '%s' "$3" | grep -Eq '^[0-9]{8}-[0-9]{6}$' || die "a sample is named YYYYMMDD-HHMMSS"
    [ -d "$here/runs/$2/samples/$3" ] || die "no sample runs/$2/samples/$3"
    rm -rf "$here/runs/$2/samples/$3"
    echo "dropped runs/$2/samples/$3"
    ;;
  all)
    pinned_bin >/dev/null || exit 1   # before any sample is dropped: no pinned binary, nothing is lost
    for r in "$here"/runs/*/; do rm -rf "$r/samples"; capture_run "$(basename "$r")"; done
    refreeze_cited
    ;;
  *) die "usage: capture.sh run <name> [--rerecord] | pin <version> | drop <run> <ts> | all" ;;
esac
