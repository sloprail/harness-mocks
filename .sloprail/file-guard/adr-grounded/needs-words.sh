#!/usr/bin/env bash
# `when` for adr-grounded's citation. Exit 0 applies it, exit 1 waives it; any
# failure applies it (fail closed). Waived ONLY when every ADR change of the
# subject (the rule is split per ADR; the whole changeset when it is not) is an
# ADR.md edit that removes exception entries and adds nothing: a legacy site
# brought into line shrinks the list, which the decision already asked for.
set -uo pipefail
payload="$(cat)"
command -v jq >/dev/null 2>&1 || exit 0
printf '%s' "$payload" | jq -e '
  (.subject.files // [.changeset.files[].path]) as $mine
  | [.changeset.files[] | select(.path as $p | ($mine | index($p)) != null) | select(.path | test("^adr/"))] as $f
  | ($f | length) > 0
  and all($f[];
      .status == "M" and (.path | endswith("/ADR.md"))
      and ([.diff | split("\n")[] | select(test("^\\+") and (test("^\\+\\+\\+") | not))] | length) == 0
      and all(.diff | split("\n")[] | select(test("^-") and (test("^---") | not));
              test("^-[[:space:]]*- ")))' >/dev/null && exit 1
exit 0
