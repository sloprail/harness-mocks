#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. Proves a _test.go file counts: a mock's test importing another
# mock is refused (the ADR bans the import whatever file holds it), and the recovery passes.
# The sandbox has no go: a `go` shim answers `go list` the way the real one does for these files. It
# lists a package's test imports only when the rule's -f template asks for TestImports/XTestImports,
# so the case fails when the rule stops asking for them.
mkdir -p "$TMPDIR/shim"
cat > "$TMPDIR/shim/go" <<'EOS'
#!/usr/bin/env bash
if [ "$1 $2" = "list -m" ]; then echo example.com/m; exit 0; fi
tmpl="$*"
echo "example.com/m/a-mock example.com/m/internal/core"
line="example.com/m/b-mock example.com/m/internal/core"
if [ -f b-mock/b_test.go ] && grep -q 'example.com/m/a-mock' b-mock/b_test.go; then
  case "$tmpl" in *TestImports*) line="$line example.com/m/a-mock" ;; esac
fi
echo "$line"
EOS
chmod +x "$TMPDIR/shim/go"
export PATH="$TMPDIR/shim:$PATH"
git init -q .
printf 'module example.com/m\n\ngo 1.21\n' > go.mod
mkdir -p a-mock b-mock internal/core
printf 'package a\n\nfunc A() {}\n' > a-mock/a.go
printf 'package b\n\nfunc B() {}\n' > b-mock/b.go
printf 'package core\n\nfunc C() {}\n' > internal/core/c.go
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "base"
BASE=$(git rev-parse HEAD)

verdict() {   # the layering outcome over BASE..HEAD
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 || true
}

# a test of b-mock imports a-mock: refused, naming the import
git checkout -q -b bad "$BASE"
printf 'package b\n\nimport (\n\t"testing"\n\n\t"example.com/m/a-mock"\n)\n\nfunc TestB(t *testing.T) { a.A() }\n' > b-mock/b_test.go
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "a test imports another mock"
verdict
jq -es 'any(.[]; .kind=="FileGuardChecked" and .rule=="layering" and .outcome=="refused" and (.reason|contains("imports example.com/m/a-mock (another mock, a-mock)")))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; echo "a test importing another mock was not refused by layering" >&2; exit 1; }

# recovery: the test imports core only, and the same range passes
printf 'package b\n\nimport (\n\t"testing"\n\n\t"example.com/m/internal/core"\n)\n\nfunc TestB(t *testing.T) { core.C() }\n' > b-mock/b_test.go
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "the test imports core"
verdict
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="layering")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; echo "a test importing core was not passed by layering" >&2; exit 1; }
