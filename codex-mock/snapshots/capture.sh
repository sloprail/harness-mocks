#!/usr/bin/env bash
# The ONLY writer of codex-mock/snapshots/ (the snapshots-read-only gate
# refuses every other write). Everything it writes is sealed; snapshots-current
# refuses a snapshot whose seal does not match, so a hand edit is caught at the
# commit even if it slipped past the gate.
#
#   capture.sh run <name>    run the scenario in runs/<name>/setup/ against the
#                            real `codex exec`, and add a sample
#   capture.sh doc <url>     freeze a doc page (e.g. https://developers.openai.com/codex/hooks):
#                            its sha256 goes in the MANIFEST; its text only into a cache
#                            under the git dir, never into the repo
#   capture.sh drop <run> <ts>  remove one sample (a bad or non-hermetic capture)
#   capture.sh all           re-capture every run and doc at the installed
#                            codex's version, and set MANIFEST.version to it
#
# A scenario is hand-authored, and only its setup/ is:
#   runs/<name>/setup/prompt.txt       the prompt
#   runs/<name>/setup/hooks.json       the user layer's $CODEX_HOME/hooks.json
#   runs/<name>/setup/project-hooks.json  optional: the project layer's <repo>/.codex/hooks.json
#   runs/<name>/setup/hook.sh          optional: the hook every event runs;
#                                      it appends its stdin to $HOOK_LOG
#   runs/<name>/setup/args             optional: extra codex exec flags, one per line
#   runs/<name>/setup/no-json          optional: run without --json, so stream.jsonl holds the
#                                      final message (stdout) and stderr.txt the progress
#   runs/<name>/setup/prepare.sh       optional: run in the scratch repo before codex, with
#                                      HOME and CODEX_HOME the run's (e.g. to lay out a plugin
#                                      marketplace and register it with `codex plugin`)
#   runs/<name>/setup/no-skip-git-check optional: run without --skip-git-repo-check
#   runs/<name>/setup/no-sandbox-bypass optional: run without --dangerously-bypass-approvals-and-sandbox
#   runs/<name>/setup/no-git           optional: run in a directory that is not a git repository
#   runs/<name>/setup/schema.json      optional: copied to the run's directory as schema.json
#                                      (what an `args` line `--output-schema schema.json` names)
#   runs/<name>/setup/env             optional: KEY=VALUE lines codex inherits on
#                                      top of the hermetic env (e.g. an outer session's)
#
# Authentication: codex has no headless login but the user's. The run's fake
# CODEX_HOME holds ONLY a symlink to the logged-in auth.json (the same trick as
# claude-mock's Keychains link): nothing is copied, and a token codex refreshes
# during the run is moved back over the original, so the login stays valid.
# The link is gone with the fake home, and a capture is refused if any token
# value reached the sample.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
manifest="$here/MANIFEST.yaml"
die() { echo "capture.sh: $*" >&2; exit 1; }
version() { yq -r '.version' "$manifest"; }
installed() { codex --version | awk '{print $2}'; }
auth_src="${CODEX_AUTH_JSON:-$HOME/.codex/auth.json}"
# The first capture creates the MANIFEST, frozen at the installed version: it is
# a snapshot file too, so nothing else may write it.
[ -f "$manifest" ] || printf 'version: "%s"\n' "$(installed)" >"$manifest"

# One capture at a time: they are real model calls on one account.
lock=/tmp/capture-codex.lock.d
acquire() { local i=0; until mkdir "$lock" 2>/dev/null; do i=$((i + 1)); [ "$i" -le 600 ] || die "another capture holds $lock"; sleep 1; done; }

# seal DIR — SEAL lists the sha256 of every other file under DIR.
# Written to SEAL.tmp and renamed only on success, so a SEAL always means a
# complete sample (the cleanup trap relies on it).
seal() { (cd "$1" && find . -type f ! -name 'SEAL*' | LC_ALL=C sort | xargs shasum -a 256 >SEAL.tmp && mv SEAL.tmp SEAL); }

# normalize — hook payloads and event-stream frames into events.jsonl: what a
# scenario did, without what differs between two captures of the same
# behaviour (ids, paths, timings, the model's own wording).
# The run's own session (thread) id is kept as <SESSION_ID> wherever it appears
# (a payload's session_id, a child's env, a command's output): that they match
# is recorded, the value itself is not.
normalize() {
  local cap="$1" sid
  sid="$(jq -r 'select(.session_id) | .session_id' "$cap/payloads.jsonl" 2>/dev/null | head -n1 || true)"
  jq -c --arg sid "${sid:-<none>}" 'walk(if type == "object" then del(.transcript_path, .cwd, .turn_id, .tool_use_id, .uuid,
          .timestamp, .duration_ms, .last_assistant_message, .model)
          elif type == "string" then gsub($sid; "<SESSION_ID>") else . end)
         | del(.tool_input.description?) | {event: "hook", hook: .hook_event_name, payload: .}' "$cap/payloads.jsonl" 2>/dev/null || true
  jq -c 'select(.type != null) | {event: "stream", type, subtype: (.item.type // null)}' "$cap/stream.jsonl" 2>/dev/null || true
}

capture_run() {
  local name="$1" run="$here/runs/$1" v ts home chome
  work="" cap=""   # global: the EXIT trap below reads them
  [ -f "$run/setup/prompt.txt" ] || die "runs/$name/setup/prompt.txt is missing: author the scenario first"
  [ -f "$auth_src" ] || die "no codex login at $auth_src: run 'codex login' first"
  v="$(version)"; [ "$(installed)" = "$v" ] || die "installed codex is $(installed), MANIFEST.version is $v: run 'capture.sh all' to move to it"
  ts="$(date -u +%Y%m%d-%H%M%S)"
  cap="$run/samples/$ts"
  [ ! -e "$cap" ] || die "a sample named $ts already exists; re-run in a second"
  work="$(cd "$(mktemp -d)" && pwd -P)"; home="$work/home"; chome="$home/.codex"   # canonical (/private/var/…), so sanitizing matches the paths codex records
  # a capture that fails part-way leaves nothing behind: no half-written sample,
  # no login link, and the lock is released
  trap 'rm -rf "$work" "$lock"; [ -f "$cap/SEAL" ] || rm -rf "$cap"' EXIT
  mkdir -p "$work/repo" "$chome" "$cap"
  mkdir -m 700 "$work/tmp"
  ln -s "$auth_src" "$chome/auth.json"   # keeps the login, nothing else
  [ -f "$run/setup/hooks.json" ] && cp "$run/setup/hooks.json" "$chome/hooks.json"
  [ -f "$run/setup/project-hooks.json" ] && mkdir -p "$work/repo/.codex" && cp "$run/setup/project-hooks.json" "$work/repo/.codex/hooks.json"
  [ -f "$run/setup/hook.sh" ] && cp "$run/setup/hook.sh" "$work/repo/hook.sh" && chmod +x "$work/repo/hook.sh"
  if [ ! -f "$run/setup/no-git" ]; then
    git -C "$work/repo" init -q && git -C "$work/repo" -c commit.gpgsign=false commit -q --allow-empty -m init
  fi
  [ -f "$run/setup/schema.json" ] && cp "$run/setup/schema.json" "$work/repo/schema.json"
  [ ! -f "$run/setup/prepare.sh" ] || (cd "$work/repo" && env HOME="$home" CODEX_HOME="$chome" TMPDIR="$work/tmp" sh "$run/setup/prepare.sh")
  bypassflag=(--dangerously-bypass-approvals-and-sandbox); [ -f "$run/setup/no-sandbox-bypass" ] && bypassflag=()
  skipflag=(--skip-git-repo-check); [ -f "$run/setup/no-skip-git-check" ] && skipflag=()
  args=(); [ -f "$run/setup/args" ] && while IFS= read -r a; do [ -n "$a" ] && args+=("$a"); done <"$run/setup/args"
  jsonflag=(--json); [ -f "$run/setup/no-json" ] && jsonflag=()
  extra=(); [ -f "$run/setup/env" ] && while IFS= read -r a; do [ -n "$a" ] && extra+=("$a"); done <"$run/setup/env"
  # Hermetic: codex starts from an EMPTY environment plus only what it needs to
  # run, so nothing of the session that runs this script (CODEX_*, another
  # harness's variables) can leak into what the capture records as the
  # harness's own behaviour. stdin is closed so exec does not wait for it. No
  # user config, skills or AGENTS.md exist in the fake home.
  local codex_bin; codex_bin="$(command -v codex)" || die "codex is not on PATH"
  set +e
  (cd "$work/repo" && env -i PATH="$PATH" HOME="$home" CODEX_HOME="$chome" USER="${USER:-}" LANG="${LANG:-en_US.UTF-8}" \
    TERM="${TERM:-dumb}" TMPDIR="$work/tmp" HOOK_LOG="$cap/payloads.jsonl" ${extra[@]+"${extra[@]}"} \
    "$codex_bin" exec ${jsonflag[@]+"${jsonflag[@]}"} ${skipflag[@]+"${skipflag[@]}"} ${bypassflag[@]+"${bypassflag[@]}"} --dangerously-bypass-hook-trust \
      -m gpt-5.6-luna -c 'model_reasoning_effort="low"' \
      ${args[@]+"${args[@]}"} "$(cat "$run/setup/prompt.txt")" </dev/null >"$cap/stream.jsonl" 2>"$cap/stderr.txt")
  echo $? >"$cap/exit.txt"
  set -e
  # a token codex refreshed replaced the link: put it back, so the login stays valid
  if [ -f "$chome/auth.json" ] && [ ! -L "$chome/auth.json" ]; then mv "$chome/auth.json" "$auth_src"; fi
  mkdir -p "$cap/transcript"
  find "$chome/sessions" -name 'rollout-*.jsonl' -type f 2>/dev/null | while IFS= read -r f; do
    # drop what the login and the vendor put there: the account's and user's ids,
    # the vendor's system prompt, encrypted reasoning, usage and rate limits
    jq -c 'select((.type // "") | test("^(world_state|token_usage_record)$") | not)
           | select(((.payload.type // "") == "token_count") | not)
           | if .type == "session_meta" then .payload |= del(.creator_user_id, .creator_account_id, .base_instructions)
             elif (.payload.type // "") == "reasoning" then .payload |= del(.encrypted_content) else . end' "$f" >"$cap/transcript/$(basename "$f")"
  done || true   # a run that never started a session (a failed resume) has no rollout
  # redact: the value of every secret-named variable a child saw
  { jq -r 'select(.hook_env) | .hook_env | to_entries[] | select(.key | test("TOKEN|SECRET|KEY|PASSWORD")) | .value' "$cap/payloads.jsonl" 2>/dev/null || true
    grep -rhoE '[A-Z0-9_]*(TOKEN|SECRET|KEY|PASSWORD)[A-Z0-9_]*=[A-Za-z0-9._/+-]{8,}' "$cap" 2>/dev/null | cut -d= -f2- || true   # no secret is fine
  } | sort -u | while IFS= read -r secret; do
    [ "${#secret}" -ge 8 ] || continue
    { grep -rlF -e "$secret" "$cap" 2>/dev/null || true; } | while IFS= read -r f; do sed -i '' -e "s#$secret#<REDACTED>#g" "$f"; done
  done
  # sanitize: the machine's paths out of everything captured
  { grep -rlF -e "$work" -e "$HOME" "$cap" 2>/dev/null || true; } | while IFS= read -r f; do
    sed -i '' -e "s#$work/repo#<RUN>#g" -e "s#$work#<TMP>#g" -e "s#$HOME#<HOME>#g" "$f"
  done
  normalize "$cap" >"$cap/events.jsonl"
  # a sample never carries an email address but Anthropic's attribution one
  if grep -rhoE '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[a-z]{2,}' "$cap" | grep -vxq 'noreply@anthropic.com'; then
    grep -rlE '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[a-z]{2,}' "$cap" | sed "s#^$cap/#  #" >&2
    die "the capture holds an email address in the files above; not sealed, and removed"
  fi
  # nor a credential, an account or a user: every token value in the login file, the
  # ids the rollout names, the user's name and any home path
  local secrets; secrets="$(jq -r '[.. | strings | select(length >= 12)] | .[]' "$auth_src" 2>/dev/null || true)"
  while IFS= read -r s; do
    [ -n "$s" ] || continue
    ! grep -rqF -e "$s" "$cap" || die "a login value reached the capture; not sealed, and removed"
  done <<<"$secrets"
  if grep -rqE -e '/Users/' -e 'creator_(user|account)_id' ${USER:+-e "$USER"} "$cap"; then
    grep -rlE -e '/Users/' -e 'creator_(user|account)_id' ${USER:+-e "$USER"} "$cap" | sed "s#^$cap/#  #" >&2
    die "the capture names a user, account or home path in the files above; not sealed, and removed"
  fi
  # a re-capture with the same events adds nothing
  for other in "$run"/samples/*/; do
    [ "$other" = "$cap/" ] && continue
    if cmp -s "$other/events.jsonl" "$cap/events.jsonl"; then
      rm -rf "$cap"; echo "same events as $(basename "$other"): no new sample"; return 0
    fi
  done
  printf 'version: %s\ncommand: codex exec %s%s%s--dangerously-bypass-hook-trust -m gpt-5.6-luna\n' "$v" "${jsonflag[*]:+--json }" "${skipflag[*]:+--skip-git-repo-check }" "${bypassflag[*]:+--dangerously-bypass-approvals-and-sandbox }" >"$run/run.yaml"
  seal "$cap"
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
  run) [ -n "${2:-}" ] || die "usage: capture.sh run <name>"; acquire; trap 'rm -rf "$lock"' EXIT; capture_run "$2" ;;
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
    acquire; trap 'rm -rf "$lock"' EXIT
    v="$(installed)"; yq -i ".version = \"$v\"" "$manifest"
    for r in "$here"/runs/*/; do rm -rf "$r/samples"; capture_run "$(basename "$r")"; done
    for u in $(yq -r '.docs // {} | keys | .[]' "$manifest"); do capture_doc "$u"; done
    ;;
  *) die "usage: capture.sh run <name> | drop <run> <ts> | doc <url> | all" ;;
esac
