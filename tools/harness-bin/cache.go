package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// settings is everything the tool reads from its environment, once, in main.
type settings struct {
	cache        string // HARNESS_BIN_CACHE, else the per-user cache dir
	npm          string // HARNESS_BIN_NPM, else npm
	cursorBase   string // HARNESS_BIN_CURSOR_BASE, else https://downloads.cursor.com/lab
	cursorScript string // HARNESS_BIN_CURSOR_INSTALL_URL, else https://cursor.com/install
	goos, goarch string
}

func (s settings) dir(h harness, v string) string { return filepath.Join(s.cache, h.name, v) }

// path is the pinned binary of h at v: it exists and reports exactly v. Anything else is an
// error, so a caller never runs a binary that is not the pinned one.
func (s settings) path(h harness, v string) (string, error) {
	if !h.valid(v) {
		return "", fmt.Errorf("%q is not an exact %s version (a pin is never a range or a tag)", v, h.name)
	}
	bin := filepath.Join(s.dir(h, v), h.bin)
	if _, err := os.Stat(bin); err != nil {
		return "", fmt.Errorf("%s %s is not installed: run tools/harness-bin install %s %s", h.name, v, h.name, v)
	}
	got, err := h.run(bin)
	if err != nil {
		return "", err
	}
	if !h.same(v, got) {
		return "", fmt.Errorf("%s reports %s, not the pinned %s: delete %s and install it again", bin, got, v, s.dir(h, v))
	}
	return bin, nil
}

// run asks the binary for its version, with auto-update off and nothing of the caller's session.
func (h harness) run(bin string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--version")
	cmd.Env = append(os.Environ(), h.noUpdate...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s --version: %w", bin, err)
	}
	return h.version(strings.TrimSpace(string(out))), nil
}

// install puts h at v into the cache (idempotent) and returns its binary. It builds in a
// scratch directory next to the target and renames it into place, so a failed install leaves
// nothing a later `path` could mistake for the pin.
func (s settings) install(h harness, v string) (string, error) {
	if bin, err := s.path(h, v); err == nil {
		return bin, nil
	}
	if !h.valid(v) {
		return "", fmt.Errorf("%q is not an exact %s version (a pin is never a range or a tag)", v, h.name)
	}
	dest := s.dir(h, v)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dest), ".tmp-"+v+"-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err := h.install(s, tmp, v); err != nil {
		return "", err
	}
	if got, err := h.run(filepath.Join(tmp, h.bin)); err != nil || !h.same(v, got) {
		return "", fmt.Errorf("installing %s %s produced %q (%v): not the pin, nothing was kept", h.name, v, got, err)
	}
	if err := os.RemoveAll(dest); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return "", err
	}
	return s.path(h, v)
}
