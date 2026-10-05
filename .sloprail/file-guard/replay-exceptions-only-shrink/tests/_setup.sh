# Shared by the cases of replay-exceptions-only-shrink (sourced, not run). The rule builds
# tools/replaycheck from the committed tree, so a case's sandbox repo needs it, a go.mod and a go.
#
# install_checker — copies tools/replaycheck (without its tests) into the sandbox repo, writes a go.mod
# when there is none, and puts a go on PATH with a cache of its own. Call it before the base commit.
install_checker() {
  local root go g
  root="$(cd "$SR_TEST_SLOPRAIL_DIR/.." && pwd)"
  mkdir -p tools/replaycheck
  for g in "$root"/tools/replaycheck/*.go; do
    case "$g" in *_test.go) ;; *) cp "$g" tools/replaycheck/ ;; esac
  done
  [ -f go.mod ] || printf 'module example.com/m\n\ngo 1.21\n' > go.mod
  go=""
  for g in /usr/local/go/bin/go /opt/homebrew/bin/go /usr/local/bin/go /usr/bin/go \
    /Users/*/.asdf/installs/golang/*/go/bin/go /opt/hostedtoolcache/go/*/x64/bin/go /root/go/bin/go; do
    [ -x "$g" ] && { go="$g"; break; }
  done
  [ -n "$go" ] || { echo "no go found: the rule builds tools/replaycheck with it" >&2; return 1; }
  mkdir -p bin
  ln -sf "$go" bin/go
  printf 'bin\nout\n' >> .gitignore
  export PATH="$PWD/bin:$PATH" GOCACHE="$PWD/../gocache.$$" GOPATH="$PWD/../gopath.$$" GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off
  mkdir -p "$GOCACHE" "$GOPATH"
}

# good_gen — a generated replay test the rule accepts (the shape of the real ones, reduced)
good_gen() {
  printf '%s\n' \
    'package e2e' \
    '' \
    'import (' \
    '	"strings"' \
    '	"testing"' \
    ')' \
    '' \
    'const flakyRuns = 3' \
    '' \
    'func replayUntilGreen(run func() (string, error), attempts int) (string, error) { return run() }' \
    '' \
    'func TestGeneratedReplay(t *testing.T) {' \
    '	for name := range notReplaying {' \
    '		_ = name' \
    '	}' \
    '	reason, listed := notReplaying["x"]' \
    '	flaky := listed && strings.HasPrefix(reason, "flaky:")' \
    '	var diff string' \
    '	var err error' \
    '	run := func() (string, error) { return "", nil }' \
    '	if flaky {' \
    '		diff, err = replayUntilGreen(run, flakyRuns)' \
    '	}' \
    '	switch {' \
    '	case flaky && (err != nil || diff != ""):' \
    '		t.Errorf("never green in %d runs, see notReplaying", flakyRuns)' \
    '	}' \
    '}'
}
