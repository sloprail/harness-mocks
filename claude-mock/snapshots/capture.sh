#!/usr/bin/env bash
# The ONLY writer of claude-mock/snapshots/ (the snapshots-read-only gate
# refuses every other write). Everything it writes is sealed; snapshots-current
# refuses a snapshot whose seal does not match, so a hand edit is caught at the
# commit even if it slipped past the gate.
#
#   capture.sh run <name> [--rerecord]
#                            run the scenario in runs/<name>/setup/ against the
#                            pinned `claude`, and add a sample. A run recorded at
#                            another version than the pin is refused (its samples
#                            would mix versions) unless --rerecord drops them first
#   capture.sh pin <version> install that exact claude (tools/harness-bin) and make it
#                            the one captures run; nothing already recorded changes
#   capture.sh doc <url>     freeze a doc page (e.g. https://code.claude.com/docs/en/hooks):
#                            its sha256 and fetch date go in the MANIFEST; its text only
#                            into a cache under the git dir, never into the repo
#   capture.sh drop <run> <ts>  remove one sample (a bad or non-hermetic capture)
#   capture.sh all           re-record every run at the pin and re-freeze every doc
#
# Two versions, kept apart: the claude a run was recorded with is the run's own (run.yaml),
# and a doc page is frozen by its own sha256 (MANIFEST docs). MANIFEST `pin` is only which
# claude this script runs. It is never the global one on PATH: tools/harness-bin installs the
# pin into a cache of its own and refuses a binary that does not report exactly it, and
# DISABLE_AUTOUPDATER=1 keeps it from replacing itself. The login is the user's, linked in.
#
# A scenario is hand-authored, and only its setup/ is:
#   runs/<name>/setup/prompt.txt       the prompt
#   runs/<name>/setup/settings.json    the project's .claude/settings.json (hooks)
#   runs/<name>/setup/prepare.sh       optional: run in the scratch repo before claude
#                                      (lays out a plugin marketplace, say)
#   runs/<name>/setup/hook.sh          optional: the hook every event runs;
#                                      it appends its stdin to $HOOK_LOG
#   runs/<name>/setup/args             optional: extra claude flags, one per line
#   runs/<name>/setup/then/<NN>/       optional later steps, run in name order in the same
#                                      repo and HOME: prompt.txt, args, and `cwd` (a directory
#                                      of the repo to run from). exit.txt has one line per step.
#   runs/<name>/setup/env              optional: KEY=VALUE lines claude inherits on
#                                      top of the hermetic env (e.g. an outer session's)
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
manifest="$here/MANIFEST.yaml"
die() { echo "capture.sh: $*" >&2; exit 1; }
root="$(cd "$here/../.." && pwd)"
pin() { yq -r '.pin // ""' "$manifest" 2>/dev/null; }
hbin() { (cd "$root" && go run ./tools/harness-bin "$@"); }
# pinned_bin — the claude binary of exactly the pin, from tools/harness-bin's cache. It dies
# unless that install exists and reports the pin: never the claude on PATH.
pinned_bin() {
  local v bin got; v="$(pin)"; [ -n "$v" ] || die "MANIFEST.yaml has no pin: run 'capture.sh pin <version>'"
  bin="$(hbin path claude "$v")" || die "claude $v is not installed for captures: run 'capture.sh pin $v'"
  got="$(DISABLE_AUTOUPDATER=1 "$bin" --version | awk '{print $1}')"
  [ "$got" = "$v" ] || die "$bin is claude $got, the pin is $v: refusing it"
  printf '%s' "$bin"
}

# seal DIR — SEAL lists the sha256 of every other file under DIR.
# Written to SEAL.tmp and renamed only on success, so a SEAL always means a
# complete sample (the cleanup trap relies on it).
seal() { (cd "$1" && find . -type f ! -name 'SEAL*' | LC_ALL=C sort | xargs shasum -a 256 >SEAL.tmp && mv SEAL.tmp SEAL); }

# normalize — hook payloads and stream frames into events.jsonl: what a
# scenario did, without what differs between two captures of the same
# behaviour (ids, paths, timings, the model's own wording).
# The run's own session id is kept as <SESSION_ID> wherever it appears (a
# payload's session_id, a child's env, a command's output), so that they match
# is recorded, and the harness's pid as <PID>: which session or
# process a value names is behaviour, the value itself is not.
normalize() {
  local cap="$1" sid pid
  sid="$(jq -r 'select(.session_id) | .session_id' "$cap/payloads.jsonl" 2>/dev/null | head -n1 || true)"
  pid="$(jq -r 'select(.hook_env.CLAUDE_PID) | .hook_env.CLAUDE_PID' "$cap/payloads.jsonl" 2>/dev/null | head -n1 || true)"
  jq -c --arg sid "${sid:-<none>}" --arg pid "${pid:-<none>}" 'walk(if type == "object" then del(.transcript_path, .cwd, .agent_id,
          .tool_use_id, .prompt_id, .uuid, .parentUuid, .timestamp, .duration_ms, .durationMs, .last_assistant_message)
          elif type == "string" then gsub($sid; "<SESSION_ID>") | gsub("\\b" + $pid + "\\b"; "<PID>") else . end)
         | del(.tool_input.description?) | {event: "hook", hook: .hook_event_name, payload: .}' "$cap/payloads.jsonl" 2>/dev/null || true
  jq -c 'select(.type != "assistant" and .type != "user")
         | {event: "stream", type, subtype: (.subtype // null)}' "$cap/stream.jsonl" 2>/dev/null || true
}

capture_run() {
  local name="$1" rerecord="${2:-}" run="$here/runs/$1" v ts home rv
  local claude_bin
  work="" cap=""   # global: the EXIT trap below reads them
  [ -f "$run/setup/prompt.txt" ] || die "runs/$name/setup/prompt.txt is missing: author the scenario first"
  v="$(pin)"; claude_bin="$(pinned_bin)" || exit 1
  rv="$(yq -r '.version // ""' "$run/run.yaml" 2>/dev/null || true)"
  if [ -n "$rv" ] && [ "$rv" != "$v" ] && ls "$run"/samples/*/ >/dev/null 2>&1; then
    [ "$rerecord" = --rerecord ] || die "runs/$name was recorded at claude $rv, the pin is $v: a sample added now would mix versions; re-record it with 'capture.sh run $name --rerecord'"
    rm -rf "$run/samples"
  fi
  ts="$(date -u +%Y%m%d-%H%M%S)"
  cap="$run/samples/$ts"
  [ ! -e "$cap" ] || die "a sample named $ts already exists; re-run in a second"
  work="$(cd "$(mktemp -d)" && pwd -P)"; home="$work/home"   # canonical (/private/var/…), so sanitizing matches the paths claude records
  # a capture that fails part-way leaves nothing behind: no half-written sample
  trap 'rm -rf "$work"; [ -f "$cap/SEAL" ] || rm -rf "$cap"' EXIT
  mkdir -p "$work/repo/.claude" "$home/Library" "$cap"
  mkdir -m 700 "$work/tmp"   # private: claude refuses a shared temp root for its per-uid dir
  ln -s "$HOME/Library/Keychains" "$home/Library/Keychains" 2>/dev/null || true   # keeps the login, nothing else
  cp "$run/setup/settings.json" "$work/repo/.claude/settings.json" 2>/dev/null || true
  [ -f "$run/setup/hook.sh" ] && cp "$run/setup/hook.sh" "$work/repo/hook.sh" && chmod +x "$work/repo/hook.sh"
  # prepare.sh lays out what a scenario needs beyond settings and a hook (a plugin
  # marketplace, say), in the scratch repo, before claude starts; it may rewrite
  # .claude/settings.json (it knows the repo's absolute path: $PWD).
  # It sees the same hermetic home as claude does, never the real one.
  [ ! -f "$run/setup/prepare.sh" ] || (cd "$work/repo" && env HOME="$home" TMPDIR="$work/tmp" CLAUDE_CODE_TMPDIR="$work/tmp" sh "$run/setup/prepare.sh")
  git -C "$work/repo" init -q && git -C "$work/repo" -c commit.gpgsign=false commit -q --allow-empty -m init
  extra=(); [ -f "$run/setup/env" ] && while IFS= read -r a; do [ -n "$a" ] && extra+=("$a"); done <"$run/setup/env"
  # Hermetic: claude starts from an EMPTY environment plus only what it needs to
  # run, so nothing of the session that runs this script (CLAUDECODE,
  # CLAUDE_CODE_SESSION_ID, CLAUDE_CODE_ENTRYPOINT, …) can leak into what the
  # capture records as the harness's own behaviour. stdin is closed so a -p run
  # does not wait for it. CLAUDE_CODE_TMPDIR too: on macOS claude ignores TMPDIR
  # for its own per-uid dir (/tmp/claude-<uid>), which the session running this
  # script shares; left there, a capture could see that session's scratchpad.
  # (A literal /tmp path stays shared: only a sandbox could stop that.)
  set +e
  # One claude invocation per step: the scenario's own setup/ is step 1, and
  # each runs/<name>/setup/then/<NN>/ is a later one, in name order, in the same
  # repo and under the same fake HOME, so a later step can resume or fork what
  # an earlier one left (the scenario fixes the session ids in each step's args).
  # A step's `cwd` file names a directory of the repo to run it from (a `symlink` file, "<name> <target>", makes <name> a symlink to <target> first); its own settings.json and hook.sh go in that directory.
  : >"$cap/stream.jsonl"; : >"$cap/stderr.txt"; : >"$cap/exit.txt"
  steps=("$run/setup"); [ -d "$run/setup/then" ] && for d in "$run/setup/then"/*/; do steps+=("${d%/}"); done
  for step in "${steps[@]}"; do
    sargs=(); [ -f "$step/args" ] && while IFS= read -r a; do [ -n "$a" ] && sargs+=("$a"); done <"$step/args"
    # a step's `symlink` file holds "<name> <target>": <name> in the repo is made a symlink
    # to the directory <target> (also in the repo), for a step that runs from <name>
    [ -f "$step/symlink" ] && { read -r lname ltarget <"$step/symlink"; mkdir -p "$work/repo/$ltarget"; ln -sfn "$work/repo/$ltarget" "$work/repo/$lname"; }
    sdir="$work/repo"; [ -f "$step/cwd" ] && sdir="$work/repo/$(cat "$step/cwd")" && { [ -L "$sdir" ] || mkdir -p "$sdir"; }
    # @TRANSCRIPTS@ in an arg is the folder this step's sessions are written to, so a step can
    # resume a session by the path of its transcript file (--resume @TRANSCRIPTS@/<id>.jsonl)
    senc="$(printf '%s' "$sdir" | sed 's#[^A-Za-z0-9]#-#g')"
    if [ "${#sargs[@]}" -gt 0 ]; then for i in "${!sargs[@]}"; do sargs[$i]="${sargs[$i]//@TRANSCRIPTS@/$home/.claude/projects/$senc}"; done; fi
    # a step's own settings.json and hook.sh are installed in its directory, which is
    # then a project of its own: the hooks of the repo root are not loaded from below it
    [ -f "$step/settings.json" ] && mkdir -p "$sdir/.claude" && cp "$step/settings.json" "$sdir/.claude/settings.json"
    [ -f "$step/hook.sh" ] && cp "$step/hook.sh" "$sdir/hook.sh" && chmod +x "$sdir/hook.sh"
    (cd "$sdir" && env -i PATH="$PATH" HOME="$home" USER="${USER:-}" LANG="${LANG:-en_US.UTF-8}" \
      TERM="${TERM:-dumb}" DISABLE_AUTOUPDATER=1 TMPDIR="$work/tmp" CLAUDE_CODE_TMPDIR="$work/tmp" HOOK_LOG="$cap/payloads.jsonl" ${extra[@]+"${extra[@]}"} \
      "$claude_bin" -p --model haiku --dangerously-skip-permissions --output-format stream-json --verbose \
        ${sargs[@]+"${sargs[@]}"} "$(cat "$step/prompt.txt")" </dev/null >>"$cap/stream.jsonl" 2>>"$cap/stderr.txt")
    echo $? >>"$cap/exit.txt"
  done
  mkdir -p "$cap/transcript"
  cp -R "$home/.claude/projects/"*/* "$cap/transcript/" 2>/dev/null || true
  # drop what the login injects, which is the account's, not the harness's
  # behaviour, and private: the user's email and organization, the account's
  # skills, agents and MCP instructions, its commit/PR attribution
  # (a sub-agent's transcript too, under <session>/subagents/)
  find "$cap/transcript" -name '*.jsonl' -type f | while IFS= read -r f; do
    jq -c 'select((.attachment.type // "") | test("^(session_context|credential_org|skill_listing|agent_listing_delta|mcp_instructions_delta|remote_session_change)$") | not)' "$f" >"$f.tmp" && mv "$f.tmp" "$f"
  done
  # redact: the value of every secret-named variable a child saw (the harness
  # hands children e.g. CLAUDE_CODE_MESSAGING_TOKEN) out of everything captured,
  # before anything is sealed or committed
  { jq -r 'select(.hook_env) | .hook_env | to_entries[] | select(.key | test("TOKEN|SECRET|KEY|PASSWORD")) | .value' "$cap/payloads.jsonl" 2>/dev/null || true
    grep -rhoE '[A-Z0-9_]*(TOKEN|SECRET|KEY|PASSWORD)[A-Z0-9_]*=[A-Za-z0-9._/+-]{8,}' "$cap" 2>/dev/null | cut -d= -f2- || true   # no secret is fine
  } | sort -u | while IFS= read -r secret; do
    [ "${#secret}" -ge 8 ] || continue
    { grep -rlF -e "$secret" "$cap" 2>/dev/null || true; } | while IFS= read -r f; do sed -i '' -e "s#$secret#<REDACTED>#g" "$f"; done
  done
  # sanitize: the machine's paths out of everything captured, and the run dir as
  # claude encodes it into a folder name (every non-alphanumeric as -)
  local enc; enc="$(printf '%s' "$work/repo" | sed 's#[^A-Za-z0-9]#-#g')"
  { grep -rlF -e "$work" -e "$HOME" -e "$enc" "$cap" 2>/dev/null || true; } | while IFS= read -r f; do
    sed -i '' -e "s#$work/repo#<RUN>#g" -e "s#$work#<TMP>#g" -e "s#$HOME#<HOME>#g" -e "s#$enc#<RUN_DIRNAME>#g" "$f"
  done
  normalize "$cap" >"$cap/events.jsonl"
  # a sample never carries an email address but Anthropic's attribution one
  if grep -rhoE '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[a-z]{2,}' "$cap" | grep -vxq 'noreply@anthropic.com'; then
    grep -rlE '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[a-z]{2,}' "$cap" | sed "s#^$cap/#  #" >&2
    die "the capture holds an email address in the files above; not sealed, and removed"
  fi
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
  local url="${1%%#*}" tmp sha cache
  url="${url%/}"; tmp="$(mktemp)"
  curl -fsSL "$url.md" -o "$tmp" || die "could not fetch $url.md"
  head -c 200 "$tmp" | grep -q '<html' && die "$url.md is not markdown"
  sha="$(shasum -a 256 "$tmp" | cut -d' ' -f1)"
  cache="$(git -C "$here" rev-parse --path-format=absolute --git-common-dir)/sloprail-doc-cache"
  mkdir -p "$cache" && mv "$tmp" "$cache/$sha.md"
  URL="$url" FETCHED="$(date -u +%F)" SHA="$sha" yq -i '.docs[strenv(URL)] = {"sha256": strenv(SHA), "fetched": strenv(FETCHED)}' "$manifest"
  echo "froze $url at sha256 $sha (text cached, not committed)"
}

case "${1:-}" in
  run) [ -n "${2:-}" ] || die "usage: capture.sh run <name> [--rerecord]"; capture_run "$2" "${3:-}" ;;
  pin)
    [ -n "${2:-}" ] || die "usage: capture.sh pin <version>"
    hbin install claude "$2" >/dev/null || die "could not install claude $2"
    V="$2" yq -i '.pin = strenv(V)' "$manifest" 2>/dev/null || { printf 'pin: "%s"\n' "$2" >"$manifest"; }
    echo "pinned claude $2"
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
  doc) [ -n "${2:-}" ] || die "usage: capture.sh doc <url>"; capture_doc "$2" ;;
  all)
    for r in "$here"/runs/*/; do rm -rf "$r/samples"; capture_run "$(basename "$r")"; done
    for u in $(yq -r '.docs // {} | keys | .[]' "$manifest"); do capture_doc "$u"; done
    ;;
  *) die "usage: capture.sh run <name> [--rerecord] | pin <version> | drop <run> <ts> | doc <url> | all" ;;
esac
