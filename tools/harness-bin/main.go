// Command harness-bin installs an exact version of a real harness CLI into a cache of its own,
// never the global install, so a recording is made by the version it says it was made by.
//
//	go run ./tools/harness-bin install <claude|codex|cursor> <version>
//	go run ./tools/harness-bin path    <claude|codex|cursor> <version>
//
// install is idempotent and prints the binary's path. path prints it too, and fails unless that
// binary exists and reports exactly <version>. Both run the binary with --version only.
//
// Authentication is not handled here and nothing is copied: capture.sh links the user's own
// login (Keychains, ~/.codex/auth.json) into its fake HOME. Auto-update is off where the harness
// has a switch (Claude: DISABLE_AUTOUPDATER=1, cursor-agent: --disable-auto-update).
//
// The cache is HARNESS_BIN_CACHE, else the per-user cache directory (shared by every worktree).
// Exit status: 0 ok, 1 the install is missing or wrong, 2 usage.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, environ())) }

func environ() settings {
	cache := os.Getenv("HARNESS_BIN_CACHE")
	if cache == "" {
		if d, err := os.UserCacheDir(); err == nil {
			cache = filepath.Join(d, "harness-mocks", "harness-bin")
		}
	}
	pick := func(name, def string) string {
		if v := os.Getenv(name); v != "" {
			return v
		}
		return def
	}
	return settings{
		cache: cache, npm: pick("HARNESS_BIN_NPM", "npm"),
		cursorBase:   pick("HARNESS_BIN_CURSOR_BASE", "https://downloads.cursor.com/lab"),
		cursorScript: pick("HARNESS_BIN_CURSOR_INSTALL_URL", "https://cursor.com/install"),
		goos:         runtime.GOOS, goarch: runtime.GOARCH,
	}
}

func run(args []string, stdout, stderr io.Writer, s settings) int {
	if len(args) != 3 || (args[0] != "install" && args[0] != "path") {
		fmt.Fprintln(stderr, "usage: harness-bin install|path <claude|codex|cursor> <version>")
		return 2
	}
	h, err := lookup(args[1])
	if err != nil {
		fmt.Fprintln(stderr, "harness-bin:", err)
		return 2
	}
	if s.cache == "" {
		fmt.Fprintln(stderr, "harness-bin: no cache directory: set HARNESS_BIN_CACHE")
		return 2
	}
	do := s.path
	if args[0] == "install" {
		do = s.install
	}
	bin, err := do(h, args[2])
	if err != nil {
		fmt.Fprintln(stderr, "harness-bin:", err)
		return 1
	}
	fmt.Fprintln(stdout, bin)
	return 0
}
