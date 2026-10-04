package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeNpm is a stand-in for npm: it "installs" <pkg>@<v> by writing a binary that prints
// <banner> (with %V for the version) when asked --version, and logs every call. A binary of
// the claude package only reports its version when auto-update is off, as a check that the
// tool runs it that way.
func fakeNpm(t *testing.T, wrong string) (npm, log string) {
	t.Helper()
	dir := t.TempDir()
	npm, log = filepath.Join(dir, "npm"), filepath.Join(dir, "calls.log")
	script := `#!/bin/sh
echo "$*" >> "` + log + `"
prefix=""; while [ $# -gt 0 ]; do [ "$1" = --prefix ] && prefix="$2"; last="$1"; shift; done
pkg="${last%@*}"; v="${last##*@}"; [ -n "` + wrong + `" ] && v="` + wrong + `"
name="${pkg##*/}"; name="${name%-code}"
mkdir -p "$prefix/node_modules/.bin"
cat > "$prefix/node_modules/.bin/$name" <<EOS
#!/bin/sh
case "$name" in
  claude) [ "\$DISABLE_AUTOUPDATER" = 1 ] && echo "$v (Claude Code)" || echo "0.0.0 (Claude Code)" ;;
  codex) echo "codex-cli $v" ;;
esac
EOS
chmod +x "$prefix/node_modules/.bin/$name"
`
	if err := os.WriteFile(npm, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return npm, log
}

func newSettings(t *testing.T, npm string) settings {
	t.Helper()
	return settings{cache: t.TempDir(), npm: npm, goos: "darwin", goarch: "arm64",
		cursorBase: "http://127.0.0.1:1", cursorScript: "http://127.0.0.1:1/install"}
}

func calls(t *testing.T, log string) int {
	t.Helper()
	b, _ := os.ReadFile(log)
	return strings.Count(string(b), "\n")
}

func TestInstallClaudeIsPinnedAndRunsWithAutoUpdateOff(t *testing.T) {
	npm, log := fakeNpm(t, "")
	s := newSettings(t, npm)
	bin, err := s.install(harnesses["claude"], "2.1.285")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(bin, s.cache) {
		t.Fatalf("the install must live in the cache %s, got %s", s.cache, bin)
	}
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "@anthropic-ai/claude-code@2.1.285") || !strings.Contains(string(b), "--prefix") {
		t.Fatalf("npm must install the exact package version under a prefix, got %q", b)
	}
	if got, err := s.path(harnesses["claude"], "2.1.285"); err != nil || got != bin {
		t.Fatalf("path after install = %q, %v", got, err)
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	npm, log := fakeNpm(t, "")
	s := newSettings(t, npm)
	for i := 0; i < 2; i++ {
		if _, err := s.install(harnesses["codex"], "0.159.3"); err != nil {
			t.Fatal(err)
		}
	}
	if n := calls(t, log); n != 1 {
		t.Fatalf("a second install of the same version must not run npm again, ran it %d times", n)
	}
}

func TestPathRefusesWhatIsNotInstalledOrNotExact(t *testing.T) {
	s := newSettings(t, "unused")
	if _, err := s.path(harnesses["codex"], "0.159.3"); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("a missing install must be refused with how to install it, got %v", err)
	}
	for _, v := range []string{"latest", "^1.2.3", "", "1.2.3-beta"} {
		if _, err := s.path(harnesses["claude"], v); err == nil || !strings.Contains(err.Error(), "exact") {
			t.Fatalf("%q is not an exact version and must be refused, got %v", v, err)
		}
	}
}

func TestPathRefusesABinaryOfAnotherVersion(t *testing.T) {
	npm, _ := fakeNpm(t, "")
	s := newSettings(t, npm)
	if _, err := s.install(harnesses["codex"], "0.159.3"); err != nil {
		t.Fatal(err)
	}
	// the pinned directory now holds another version (an auto-update, a hand edit)
	if err := os.Rename(s.dir(harnesses["codex"], "0.159.3"), s.dir(harnesses["codex"], "0.160.0")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.path(harnesses["codex"], "0.160.0"); err == nil || !strings.Contains(err.Error(), "not the pinned") {
		t.Fatalf("a binary that reports another version must be refused, got %v", err)
	}
}

func TestAnInstallThatYieldsTheWrongVersionLeavesNothing(t *testing.T) {
	npm, _ := fakeNpm(t, "9.9.9")
	s := newSettings(t, npm)
	if _, err := s.install(harnesses["codex"], "0.159.3"); err == nil {
		t.Fatal("npm produced 9.9.9 for a pin of 0.159.3: the install must fail")
	}
	if _, err := os.Stat(s.dir(harnesses["codex"], "0.159.3")); err == nil {
		t.Fatal("a wrong-version install must not be kept in the cache")
	}
}

func tarball(t *testing.T, version string, extra ...tar.Header) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := "#!/bin/sh\necho " + version + "\n"
	hs := append([]tar.Header{{Name: "dist-package/cursor-agent", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}}, extra...)
	for _, h := range hs {
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			content := body
			if h.Name != "dist-package/cursor-agent" {
				content = strings.Repeat("x", int(h.Size))
			}
			tw.Write([]byte(content))
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func cursorServer(t *testing.T, built map[string]string, install string) settings {
	t.Helper()
	mux := http.NewServeMux()
	for build, ver := range built {
		mux.HandleFunc("/lab/"+build+"/darwin/arm64/agent-cli-package.tar.gz", func(w http.ResponseWriter, _ *http.Request) {
			w.Write(tarball(t, ver))
		})
	}
	mux.HandleFunc("/install", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(install)) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	s := newSettings(t, "unused")
	s.cursorBase, s.cursorScript = srv.URL+"/lab", srv.URL+"/install"
	return s
}

func TestCursorInstallsAKnownDateFromItsVersionedDownload(t *testing.T) {
	s := cursorServer(t, map[string]string{"2026.09.28-64d2043": "2026.09.28-64d2043"}, "")
	bin, err := s.install(harnesses["cursor"], "2026.09.28")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(bin, s.cache) {
		t.Fatalf("cursor-agent must install into the cache, got %s", bin)
	}
	if _, err := s.path(harnesses["cursor"], "2026.09.28"); err != nil {
		t.Fatalf("the bare date pins any build of it: %v", err)
	}
	if _, err := s.path(harnesses["cursor"], "2026.09.28-aaaaaaa"); err == nil {
		t.Fatal("a pin that names another build must be refused")
	}
}

func TestCursorResolvesTheCurrentBuildFromTheInstallerScript(t *testing.T) {
	script := `FINAL_DIR="$HOME/.local/share/cursor-agent/versions/2026.11.02-abc1234"`
	s := cursorServer(t, map[string]string{"2026.11.02-abc1234": "2026.11.02-abc1234"}, script)
	if _, err := s.install(harnesses["cursor"], "2026.11.02"); err != nil {
		t.Fatal(err)
	}
}

func TestCursorRefusesADateWhoseBuildIsUnknown(t *testing.T) {
	s := cursorServer(t, nil, `versions/2026.11.02-abc1234`)
	_, err := s.install(harnesses["cursor"], "2026.10.15")
	if err == nil || !strings.Contains(err.Error(), "no build hash is known") {
		t.Fatalf("an unknown build must be refused honestly, got %v", err)
	}
}

func TestUntarRefusesAnEntryOutsideTheDirectory(t *testing.T) {
	data := tarball(t, "v", tar.Header{Name: "dist-package/../../escape", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
	dir := t.TempDir()
	if err := untar(bytes.NewReader(data), dir); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("a path-traversing entry must be refused, got %v", err)
	}
}

func TestUntarRefusesASymlinkThatLeavesTheDirectory(t *testing.T) {
	for _, target := range []string{"/etc/passwd", "../../outside", "a/../../../outside"} {
		data := tarball(t, "v", tar.Header{Name: "dist-package/link", Linkname: target, Mode: 0o777, Typeflag: tar.TypeSymlink})
		if err := untar(bytes.NewReader(data), t.TempDir()); err == nil || !strings.Contains(err.Error(), "escapes") {
			t.Fatalf("symlink to %q must be refused, got %v", target, err)
		}
	}
	data := tarball(t, "v", tar.Header{Name: "dist-package/bin/link", Linkname: "../lib/x", Mode: 0o777, Typeflag: tar.TypeSymlink})
	if err := untar(bytes.NewReader(data), t.TempDir()); err != nil {
		t.Fatalf("an in-tree symlink is fine, got %v", err)
	}
}

func TestUsage(t *testing.T) {
	var out, errb bytes.Buffer
	for _, args := range [][]string{{}, {"install", "claude"}, {"remove", "claude", "1.0.0"}, {"path", "vim", "1.0.0"}} {
		if code := run(args, &out, &errb, settings{cache: t.TempDir()}); code != 2 {
			t.Fatalf("%v: exit %d, want 2", args, code)
		}
	}
	if code := run([]string{"path", "claude", "1.0.0"}, &out, &errb, settings{cache: t.TempDir()}); code != 1 {
		t.Fatalf("a missing install exits 1, got %d", code)
	}
}
