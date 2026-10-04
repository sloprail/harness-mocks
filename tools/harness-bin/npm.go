package main

import (
	"fmt"
	"os/exec"
)

// npmInstaller installs pkg at exactly v into dir/node_modules, away from any global prefix.
// Install scripts run: the Claude Code package uses one to put its native binary in place.
func npmInstaller(pkg string) func(settings, string, string) error {
	return func(s settings, dir, v string) error {
		cmd := exec.Command(s.npm, "install", "--prefix", dir, "--no-save", "--no-package-lock",
			"--no-audit", "--no-fund", "--loglevel=error", pkg+"@"+v)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("npm install %s@%s: %w\n%s", pkg, v, err, out)
		}
		return nil
	}
}
