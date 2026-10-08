package main

import (
	"fmt"
	"regexp"
	"strings"
)

// A harness is a real agent CLI the snapshots are recorded with. Its pinned install lives in
// <cache>/<name>/<version>/ and is never the user's global one.
type harness struct {
	name string
	// bin is the executable's path inside an install directory.
	bin string
	// noUpdate is what keeps the pinned binary from replacing itself: environment assignments
	// for the process that runs it. (cursor-agent has a flag instead; see capture.sh.)
	noUpdate []string
	// version reads the harness's own version out of `<bin> --version`.
	version func(out string) string
	// valid says whether v is an exact version this harness can be pinned at.
	valid func(v string) bool
	// install puts version v into the empty directory dir.
	install func(s settings, dir, v string) error
}

var (
	dotted = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)
	// cursor-agent is released by date; a build is that date and a commit hash (2026.09.28-64d2043).
	cursorBuild = regexp.MustCompile(`^[0-9]{4}\.[0-9]{2}\.[0-9]{2}(-[0-9a-f]{7,})?$`)
)

var harnesses = map[string]harness{
	"claude": {
		name: "claude", bin: "node_modules/.bin/claude",
		noUpdate: []string{"DISABLE_AUTOUPDATER=1"},
		version:  func(out string) string { return field(out, 0) }, // "2.1.285 (Claude Code)"
		valid:    dotted.MatchString,
		install:  npmInstaller("@anthropic-ai/claude-code"),
	},
	"codex": {
		name: "codex", bin: "node_modules/.bin/codex",
		// codex has no switch for it: an npm install is updated by npm alone, and `codex exec`
		// only ever prints an upgrade notice.
		version: func(out string) string { return field(out, 1) }, // "codex-cli 0.159.3"
		valid:   dotted.MatchString,
		install: npmInstaller("@openai/codex"),
	},
	"cursor": {
		name: "cursor", bin: "cursor-agent",
		version: func(out string) string { return field(out, 0) }, // "2026.10.01-e373342"
		valid:   cursorBuild.MatchString,
		install: installCursor,
	},
}

func field(out string, i int) string {
	f := strings.Fields(out)
	if i >= len(f) {
		return ""
	}
	return f[i]
}

func lookup(name string) (harness, error) {
	h, ok := harnesses[name]
	if !ok {
		return harness{}, fmt.Errorf("unknown harness %q (claude, codex, cursor)", name)
	}
	return h, nil
}

// same reports whether the version a binary printed is the pinned one. A cursor pin may be the
// bare date, which matches any build of that date.
func (h harness) same(pinned, got string) bool {
	if got == pinned {
		return true
	}
	return h.name == "cursor" && !strings.Contains(pinned, "-") && strings.HasPrefix(got, pinned+"-")
}
