#!/usr/bin/env bash
# `when` for adr-grounded's citation. Exit 0 applies it, exit 1 waives it; any
# failure applies it (fail closed). Waived ONLY when every ADR change in the
# range is an ADR.md edit that removes exception entries and adds nothing: a
# legacy site brought into line shrinks the list, which the decision already
# asked for.
set -uo pipefail
payload="$(cat)"
command -v jq >/dev/null 2>&1 || exit 0
printf '%s' "$payload" | jq -e '
  [.changeset.files[] | select(.path | test("^\\.sloprail/file-guard/adr-"))] as $f
  | ($f | length) > 0
  and all($f[];
      .status == "M" and (.path | endswith("/ADR.md"))
      and ([.diff | split("\n")[] | select(test("^\\+") and (test("^\\+\\+\\+") | not))] | length) == 0
      and all(.diff | split("\n")[] | select(test("^-") and (test("^---") | not));
              test("^-[[:space:]]*- ")))' >/dev/null && exit 1
exit 0
