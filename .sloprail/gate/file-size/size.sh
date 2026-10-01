#!/usr/bin/env bash
# The gate half of adr/file-size: refuse a Write/Edit that would leave a Go file
# over its limit, before it lands. The rule is ../../file-guard/file-size/size-lib.sh.
#
# A write whose result the engine cannot compute (a sed, a git apply) arrives
# with resultKnown false: this gate lets it through on purpose, because the
# file-guard of the same name judges the committed bytes at Stop — the backstop
# that does not depend on a prediction. Refusing it here would block every
# shell edit to a Go file for a size the commit check will see anyway.
set -uo pipefail

payload="$(cat)"

refuse() {
  jq -n --arg r "$1" '{reason: $r}'
  exit 1
}

kind="$(printf '%s' "$payload" | jq -r '.event.kind // ""')"
case "$kind" in
  PreFileCreate | PreFileUpdate) ;;
  *) refuse "gate/file-size got an unexpected event kind '$kind'; it only judges file writes" ;;
esac
[ "$(printf '%s' "$payload" | jq -r '.event.resultKnown // false')" = "true" ] || exit 0
path="$(printf '%s' "$payload" | jq -r '.event.path // ""')"
[ -n "$path" ] || refuse "the write named no path, so its size could not be checked"
content="$(printf '%s' "$payload" | jq -r '.event.newContent // ""')"

lib_dir="${SR_GUARDRAIL_DIR:-.}/../../file-guard/file-size"
. "$lib_dir/size-lib.sh"
size_load
p="$(size_problem "$path" "$content")"
[ -z "$p" ] && exit 0
refuse "adr/file-size (Go files stay small): $p"
