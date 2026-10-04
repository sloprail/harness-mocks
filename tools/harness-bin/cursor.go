package main

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// cursorBuilds maps a release date to its build hash. cursor-agent publishes every build at a
// URL that carries the hash, but only the current build's hash is published (in cursor.com/install);
// an older one is known only from what was recorded. Add a line when a version is pinned.
var cursorBuilds = map[string]string{
	"2026.09.28": "64d2043",
	"2026.10.01": "e373342",
}

var installedBuild = regexp.MustCompile(`versions/([0-9]{4}\.[0-9]{2}\.[0-9]{2})-([0-9a-f]{7,})`)

// cursorFull is the dated-and-hashed build for v: v itself when it has the hash, else from the
// table, else from the installer script when v is the current release.
func cursorFull(s settings, v string) (string, error) {
	if strings.Contains(v, "-") {
		return v, nil
	}
	if h, ok := cursorBuilds[v]; ok {
		return v + "-" + h, nil
	}
	if body, err := get(s.cursorScript); err == nil {
		if m := installedBuild.FindStringSubmatch(string(body)); m != nil && m[1] == v {
			return v + "-" + m[2], nil
		}
	}
	return "", fmt.Errorf("cursor %s: no build hash is known for that date; pass the full build (%s-<hash>) "+
		"or add it to cursorBuilds (only the current build's hash is published, in %s)", v, v, s.cursorScript)
}

func get(url string) ([]byte, error) {
	r, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, r.Status)
	}
	return io.ReadAll(r.Body)
}

// installCursor downloads the versioned package the official installer would, but into dir
// and without touching ~/.local/bin or ~/.local/share/cursor-agent.
func installCursor(s settings, dir, v string) error {
	full, err := cursorFull(s, v)
	if err != nil {
		return err
	}
	osName := map[string]string{"darwin": "darwin", "linux": "linux"}[s.goos]
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[s.goarch]
	if osName == "" || arch == "" {
		return fmt.Errorf("cursor-agent has no package for %s/%s", s.goos, s.goarch)
	}
	url := fmt.Sprintf("%s/%s/%s/%s/agent-cli-package.tar.gz", s.cursorBase, full, osName, arch)
	r, err := http.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, r.Status)
	}
	return untar(r.Body, dir)
}
