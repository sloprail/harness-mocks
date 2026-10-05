#!/usr/bin/env bash
set -euo pipefail

# A rule refuses when it cannot work out what to check; it never passes on a failed lookup (#204).
# The other side of that: an ADR at the range's base whose frontmatter is not a mapping (a scalar) links no rule and
# is skipped, as load_adrs reads it (as {}); it is not a failed lookup. Only a real jq failure refuses.
# The CI path, no agent turn: `sr-checks run` judges a committed range with the project's rules.
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" "$TMPDIR/shim" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
git init -q .
c() { git add -A && git -c user.name=t -c user.email=t@t commit -q -m "$1"; }
mkdir -p adr/modules-cover-code internal/a internal/legacy
# at the base the ADR's frontmatter is a scalar, not a mapping
printf -- '---\njust a note\n---\n# Every piece of code belongs to a module\n' >adr/modules-cover-code/ADR.md
printf 'concern: "a"\nhome: ["internal/a/**"]\napi: ["internal/a"]\n' >internal/a/module.yaml
printf 'package a\n' >internal/a/a.go; printf 'package legacy\n' >internal/legacy/old.go
c base; BASE=$(git rev-parse HEAD)
# the range gives the ADR its real frontmatter, with an exception the base did not have
printf -- '---\nconcern: coverage\nsloprails: [file-guard/module-coverage]\nspace: ["internal/**"]\nexceptions: ["internal/legacy/**"]\n---\n# Every piece of code belongs to a module\n' >adr/modules-cover-code/ADR.md
c "the ADR is linked to the rule"
: > "$SR_EVENTS_FILE"
sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?   # (the range is refused: only the reason is asserted)
# it is judged on its merits (the exception is new and covers a file the base did not except), not refused as a failed lookup
jq -es 'any(.[]; .kind=="FileGuardChecked" and .rule=="module-coverage" and .outcome=="refused" and (.reason|contains("exceptions grew: '"'"'internal/legacy/**'"'"' is new")))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; echo "module-coverage did not judge a range whose base ADR has scalar frontmatter" >&2; exit 1; }
if jq -es 'any(.[]; .rule=="module-coverage" and .outcome=="refused" and (.reason|contains("could not be read for its links")))' "$SR_EVENTS_FILE" >/dev/null; then
  jq -c . "$SR_EVENTS_FILE" >&2; echo "a scalar-frontmatter base ADR was refused as a failed lookup" >&2; exit 1
fi
