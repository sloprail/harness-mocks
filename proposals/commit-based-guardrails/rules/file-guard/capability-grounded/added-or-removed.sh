#!/usr/bin/env bash
# `when`: exit 0 (words required) when the changeset adds or deletes a
# capability file; exit 1 (waived) otherwise. Any failure requires them.
set -uo pipefail
payload="$(cat)"
command -v jq >/dev/null 2>&1 || exit 0
printf '%s' "$payload" | jq -e 'any(.changeset.files[]; (.path | test("^spec/capabilities/")) and (.status == "A" or .status == "D"))' >/dev/null && exit 0
exit 1
