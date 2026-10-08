package runner

import (
	"github.com/sloprail/harness-mocks/cursor-mock/internal/childenv"
	"github.com/sloprail/harness-mocks/cursor-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/procexec"
)

// hookEnv is the environment of a hook command: the harness's own, with the
// facts Cursor gives a hook (recorded: runs/subprocess-session-env,
// runs/nested-session-env).
func (s *session) hookEnv() []string {
	return procexec.Env(s.cfg.Environ,
		childenv.HookIdentity(s.cfg.Dir, s.common().TranscriptPath, s.cfg.Version), childenv.HookDefaults(s.cfg.Dir, s.cfg.Version))
}

// common is what every hook payload carries now: the transcript path only once
// the conversation has a transcript.
func (s *session) common() hooks.Common {
	c := hooks.Common{SessionID: s.id, Dir: s.cfg.Dir, Version: s.cfg.Version, Model: s.cfg.Model, Generation: s.gen}
	if s.named && s.tr.exists() {
		c.TranscriptPath = s.tr.path
	}
	return c
}
