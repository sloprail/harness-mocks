# Shared by the cases of tests-fail-on-missing-tool (sourced, not run). The rule builds tools/skipcheck from the
# committed tree, so a case's sandbox repo needs it, a go.mod and a go; and it judges a range of commits, so a
# case commits a base and then a change.
#
# install_checker — copies tools/skipcheck (without its tests) into the sandbox repo, writes a go.mod when there
# is none, and puts a go on PATH with a cache of its own. Call it before the base commit.
install_checker() {
  local root go g
  root="$(cd "$SR_TEST_SLOPRAIL_DIR/.." && pwd)"
  mkdir -p tools/skipcheck
  for g in "$root"/tools/skipcheck/*.go; do
    case "$g" in *_test.go) ;; *) cp "$g" tools/skipcheck/ ;; esac
  done
  [ -f go.mod ] || printf 'module example.com/m\n\ngo 1.21\n' > go.mod
  go="$(command -v go 2>/dev/null || true)"
  if [ -z "$go" ]; then
    for g in "${GOROOT:-/nonexistent}/bin/go" "${RUNNER_TOOL_CACHE:-/nonexistent}"/go/*/x64/bin/go "${RUNNER_TOOL_CACHE:-/nonexistent}"/go/*/arm64/bin/go \
      /usr/local/go/bin/go /opt/homebrew/bin/go /usr/local/bin/go /usr/bin/go \
      /Users/*/.asdf/installs/golang/*/go/bin/go /opt/hostedtoolcache/go/*/x64/bin/go /root/go/bin/go; do
      [ -x "$g" ] && { go="$g"; break; }
    done
  fi
  # a missing go is an error of the case, never a pass: the rule cannot build its checker without it
  [ -n "$go" ] || { echo "no go found: the rule builds tools/skipcheck with it, so this case cannot run" >&2; return 1; }
  mkdir -p bin
  ln -sf "$go" bin/go
  printf 'bin\nout\n' >> .gitignore
  export PATH="$PWD/bin:$PATH" GOCACHE="$PWD/../gocache.$$" GOPATH="$PWD/../gopath.$$" GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off
  mkdir -p "$GOCACHE" "$GOPATH"
}

# branch_with NAME FILE-CONTENT — a branch from BASE whose pkg/tool_test.go is FILE-CONTENT
branch_with() {
  git checkout -q -b "$1" "$BASE"
  printf '%s\n' "$2" > pkg/tool_test.go
  git add -A && git -c user.name=t -c user.email=t@t commit -q -m "$1"
}
