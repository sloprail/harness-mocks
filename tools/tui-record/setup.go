package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var versionWord = regexp.MustCompile(`[0-9]+(\.[0-9]+)+`)

// checkVersion refuses a binary that is not the pinned harness: it must be an absolute path (never
// a name looked up on PATH) and report exactly the version the caller pinned. Only --version is run.
func checkVersion(bin, want string) error {
	if !filepath.IsAbs(bin) {
		return fmt.Errorf("--bin %q is not an absolute path: the pinned binary is never looked up on PATH", bin)
	}
	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s --version: %v", bin, err)
	}
	if got := versionWord.FindString(string(out)); got != want {
		return fmt.Errorf("%s is version %q, the pin is %q: refusing it", bin, got, want)
	}
	return nil
}

// environment is everything the harness starts with: nothing is inherited from the caller but PATH
// (the hook scripts' own tools), so what ran the recording cannot leak into what it records.
func environment(c *Config) []string {
	env := []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + c.Home, "TMPDIR=" + c.Tmp, "USER=" + os.Getenv("USER"),
		"LANG=en_US.UTF-8", "TERM=xterm-256color", "HOOK_LOG=" + c.HookLog,
	}
	return append(env, c.Env...)
}

// lay puts the login into the scratch home and nothing else of the user's: a symlink to, or a copy
// of, only the file the harness keeps its login in. Each spec is <path under home>=<source>.
func lay(home string, specs []string, link bool) error {
	for _, spec := range specs {
		rel, src, ok := strings.Cut(spec, "=")
		if !ok || rel == "" || src == "" || filepath.IsAbs(rel) || strings.Contains(rel, "..") {
			return fmt.Errorf("%q is not <path under home>=<source>", spec)
		}
		dst := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return err
		}
		if link {
			if err := os.Symlink(src, dst); err != nil {
				return err
			}
			continue
		}
		b, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o600); err != nil {
			return err
		}
	}
	return nil
}
