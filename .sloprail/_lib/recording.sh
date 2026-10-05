#!/usr/bin/env bash
# Is one recorded sample clean, and recorded in the mode the mock imitates? Source it; do not run it.
# Shared by file-guard/snapshots-current (every committed sample) and each harness's capture.sh
# (a new sample, before it is sealed), so a capture that would fail the rule is never sealed.
#
#   recording_problems HARNESS RUN_NAME RUN_DIR SAMPLE_DIR   one line per problem on stdout
#
# A sample is not clean when
#   - a line of its side-channel log (payloads.jsonl: what the hook scripts append to $HOOK_LOG)
#     is not JSON: a hook that wrote something else, or two writers interleaved, lost an event
#   - the harness printed an error line: any stderr.txt line but the harness's benign banner, or
#     an error frame in the stream (an `error` / `turn.failed` frame, a `result` with is_error)
# and its run is not in the mode the mock imitates when run.yaml's command lacks the flags that
# make a non-interactive, structured-output run (below), or the scenario's setup/args or the
# command switches a mode on or off (codex --enable / --disable).
#
# A scenario that legitimately records one of these (a hook that blocks a tool makes codex log an
# ERROR line; a text-output run) declares it in file-guard/snapshots-current/expected.yaml,
# keyed "<harness>/<run>": `stderr` / `stream` (an ERE every such line must match), `unparsed`
# (how many non-JSON side-channel lines are expected), `mode` (why the run is a mode variant).
# The declaration is a rule file: changing it is a change to the project's rules.

_REC_LIB="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RECORDING_EXPECTED="${RECORDING_EXPECTED:-$_REC_LIB/../file-guard/snapshots-current/expected.yaml}"

# the flags a run's command must carry, per harness: print mode with structured output
_rec_required_flags() {
  case "$1" in
    claude) echo '-p --output-format stream-json' ;;
    codex) echo 'exec --json' ;;
    cursor) echo '-p --output-format stream-json' ;;
  esac
}
# flags no scenario may switch the mode with (setup/args), per harness
_rec_mode_switches() { case "$1" in codex) echo '--enable --disable' ;; esac; }
# stderr lines that are the harness's banner, not an error
# stream frames that are the harness's notice of a flag the capture passes, not an error
_rec_benign_stream() { case "$1" in codex) echo '"type":"error","message":"`--dangerously-bypass-hook-trust` is enabled' ;; *) echo '^$' ;; esac; }
_rec_benign_stderr() { case "$1" in codex) echo '^Reading additional input from stdin\.\.\.$' ;; *) echo '^$' ;; esac; }

_rec_expected() {   # KEY FIELD -> the declared value, or empty
  [ -f "$RECORDING_EXPECTED" ] || return 0
  K="$1" F="$2" yq -r '.[strenv(K)][strenv(F)] // ""' "$RECORDING_EXPECTED" 2>/dev/null
}

recording_problems() {
  local h="$1" name="$2" run="$3" s="$4" key="$1/$2" n line
  local exp_stderr exp_stream exp_unparsed exp_mode
  exp_stderr="$(_rec_expected "$key" stderr)"; exp_stream="$(_rec_expected "$key" stream)"
  exp_unparsed="$(_rec_expected "$key" unparsed)"; exp_mode="$(_rec_expected "$key" mode)"

  # the side-channel log: every line parses as JSON
  if [ -s "$s/payloads.jsonl" ]; then
    n="$(jq -Rr 'try (fromjson | empty) catch "bad"' "$s/payloads.jsonl" 2>/dev/null | grep -c bad)"
    if [ "${n:-0}" -ne "${exp_unparsed:-0}" ]; then
      echo "run $key sample $(basename "$s"): payloads.jsonl (the hook log) has $n line(s) that are not JSON${exp_unparsed:+, $exp_unparsed expected}: a hook wrote something else or two writers interleaved; fix the scenario's hook, or declare 'unparsed: $n' for it in file-guard/snapshots-current/expected.yaml"
    fi
  fi

  # harness error lines on stderr
  if [ -s "$s/stderr.txt" ]; then
    while IFS= read -r line; do
      [ -n "$line" ] || continue
      printf '%s' "$line" | grep -Eq "$(_rec_benign_stderr "$h")" && continue
      [ -n "$exp_stderr" ] && printf '%s' "$line" | grep -Eq -e "$exp_stderr" && continue
      echo "run $key sample $(basename "$s"): the harness wrote an error line to stderr: ${line:0:160}; the run is not clean (re-record it, or declare the line with 'stderr: <regex>' for $key in file-guard/snapshots-current/expected.yaml)"
    done <"$s/stderr.txt"
  fi
  # ... and error frames in the stream
  if [ -s "$s/stream.jsonl" ]; then
    while IFS= read -r line; do
      [ -n "$line" ] || continue
      printf '%s' "$line" | grep -Eq -e "$(_rec_benign_stream "$h")" && continue
      [ -n "$exp_stream" ] && printf '%s' "$line" | grep -Eq -e "$exp_stream" && continue
      echo "run $key sample $(basename "$s"): the stream has an error frame: ${line:0:160}; the run is not clean (re-record it, or declare it with 'stream: <regex>' for $key in file-guard/snapshots-current/expected.yaml)"
    done < <(jq -Rc 'fromjson? | select(type == "object")
        | select(.type == "error" or .type == "turn.failed" or (.item.type? == "error") or (.type == "result" and .is_error == true))' "$s/stream.jsonl" 2>/dev/null)
  fi

  # the mode: what the mock imitates is a print-mode run with structured output
  if [ -z "$exp_mode" ] && [ -f "$run/run.yaml" ]; then
    local cmd flag; cmd=" $(yq -r '.command // ""' "$run/run.yaml" 2>/dev/null) "
    for flag in $(_rec_required_flags "$h"); do
      case "$cmd" in *" $flag "*) ;; *) echo "run $key: run.yaml's command lacks '$flag': the mock imitates the non-interactive structured-output mode, so the run was recorded in another mode (re-record it, or declare 'mode: <why>' for $key in file-guard/snapshots-current/expected.yaml)" ;; esac
    done
    for flag in $(_rec_mode_switches "$h"); do
      if grep -qxF -e "$flag" "$run/setup/args" 2>/dev/null || case "$cmd" in *" $flag "*) true ;; *) false ;; esac; then
        echo "run $key switches the mode with $flag (setup/args): the mock imitates the default mode, so this recording is another mode's (declare 'mode: <why>' for $key in file-guard/snapshots-current/expected.yaml if that variant is the point)"
      fi
    done
  fi
}
