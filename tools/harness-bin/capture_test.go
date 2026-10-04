package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// captureAll runs <harness>-mock/snapshots/capture.sh run x in a scratch tree whose MANIFEST pins
// pin and whose cache holds a fake binary (printing banner) under installedAs. The one scenario
// has a prompt and nothing else, and the "harness" is the fake: no model is called, and a run
// that gets past pinned_bin fails later on for want of anything real to record.
func captureAll(t *testing.T, harness, pin, installedAs, banner string) (string, error) {
	t.Helper()
	for _, tool := range []string{"yq", "go"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " is not installed")
		}
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, f := range []string{"go.mod", "go.sum", "tools", "internal"} {
		if err := os.Symlink(filepath.Join(repo, f), filepath.Join(root, f)); err != nil {
			t.Fatal(err)
		}
	}
	snap := filepath.Join(root, harness+"-mock", "snapshots")
	if err := os.MkdirAll(snap, 0o755); err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(filepath.Join(repo, harness+"-mock", "snapshots", "capture.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snap, "capture.sh"), script, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snap, "MANIFEST.yaml"), []byte("pin: \""+pin+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	h := harnesses[harness]
	bin := filepath.Join(cache, h.name, installedAs, h.bin)
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho '"+banner+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(snap, "runs", "x", "setup"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snap, "runs", "x", "setup", "prompt.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join(snap, "capture.sh"), "run", "x")
	cmd.Env = append(os.Environ(), "HARNESS_BIN_CACHE="+cache)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// A cursor pin may be the build (`2026.09.28-64d2043`) harness-bin installs and verifies; the
// capture must accept the very binary harness-bin accepts, not strip its hash and compare again.
func TestCaptureAcceptsACursorPinThatNamesTheBuild(t *testing.T) {
	for _, pin := range []string{"2026.09.28-64d2043", "2026.09.28"} {
		out, _ := captureAll(t, "cursor", pin, pin, "2026.09.28-64d2043")
		if strings.Contains(out, "refusing") || strings.Contains(out, "cannot be used") || strings.Contains(out, "reports") {
			t.Errorf("pin %s: capture refused the binary harness-bin accepts:\n%s", pin, out)
		}
	}
}

// What is installed but reports another version is a version mismatch, not "not installed".
func TestCaptureSaysAVersionMismatchIsOne(t *testing.T) {
	cases := []struct{ harness, pin, banner string }{
		{"cursor", "2026.09.28-64d2043", "2026.09.28-aaaaaaa"},
		{"claude", "2.1.285", "2.1.999 (Claude Code)"},
		{"codex", "0.159.3", "codex-cli 0.160.0"},
	}
	for _, c := range cases {
		out, err := captureAll(t, c.harness, c.pin, c.pin, c.banner)
		if err == nil {
			t.Errorf("%s: a binary of another version was accepted\n%s", c.harness, out)
			continue
		}
		if !strings.Contains(out, "not the pinned") || strings.Contains(out, "is not installed") {
			t.Errorf("%s: the refusal must say the version differs, not that it is not installed:\n%s", c.harness, out)
		}
	}
}

func TestCaptureSaysWhenNothingIsInstalled(t *testing.T) {
	out, err := captureAll(t, "claude", "2.1.285", "2.1.1", "2.1.1 (Claude Code)")
	if err == nil || !strings.Contains(out, "is not installed") {
		t.Errorf("want a not-installed refusal, got %v\n%s", err, out)
	}
}
