package runner

import (
	"errors"
	"fmt"
	"os"

	"github.com/sloprail/harness-mocks/codex-mock/internal/session"
)

// sessionHome is the directory the session's files go in: CODEX_HOME, or for an ephemeral session a
// scratch directory that cleanup removes (the script reads the session so far from a file, and
// nothing is kept; recorded: runs/ephemeral-no-transcript). An ephemeral session cannot resume or
// fork one: that is refused, not ignored.
func sessionHome(cfg Config) (home string, cleanup func(), err error) {
	if !cfg.Ephemeral {
		return cfg.CodexHome, func() {}, nil
	}
	if cfg.Resume != "" || cfg.ForkFrom != "" {
		return "", nil, errors.New("codex-mock: --ephemeral with resume or fork is not implemented by the mock: it is refused rather than ignored")
	}
	scratch, err := os.MkdirTemp("", "codex-mock-ephemeral-*")
	if err != nil {
		return "", nil, fmt.Errorf("codex-mock: cannot make the ephemeral session's scratch directory: %w", err)
	}
	return scratch, func() { os.RemoveAll(scratch) }, nil
}

// transcriptOf is the transcript path the session's hooks are told: the rollout's, and for an
// ephemeral session none, which the payloads say as null.
func transcriptOf(cfg Config, rollout *session.File) string {
	if cfg.Ephemeral {
		return ""
	}
	return rollout.Path
}
