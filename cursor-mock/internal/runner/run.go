package runner

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/procexec"
	"github.com/sloprail/harness-mocks/internal/turnloop"
)

// session is one run's state.
type session struct {
	cfg     Config
	id      string
	tr      *transcript
	hooks   *hooks.Hooks
	result  []byte // the script's result frame, printed last
	started time.Time
}

// Run plays one run: sessionStart hooks, the script's turns until its result,
// sessionEnd hooks, and then the result frame, last on the stream.
func Run(ctx context.Context, cfg Config) error {
	s := &session{cfg: cfg, id: newID(), started: time.Now()}
	var err error
	if s.tr, err = newTranscript(cfg.Home, cfg.Dir, s.id); err != nil {
		return fmt.Errorf("cursor-mock: %w", err)
	}
	conf, err := hooks.Load(cfg.Dir)
	if err != nil {
		return fmt.Errorf("cursor-mock: %w", err)
	}
	s.hooks = &hooks.Hooks{Config: conf, Dir: cfg.Dir, Env: procexec.Env(cfg.Environ, nil, nil), Common: s.common}
	s.tr.user(cfg.Prompt)
	s.hooks.Fire(ctx, hooks.SessionStart, map[string]any{"is_background_agent": false})
	runErr := turnloop.Run[pending](ctx, s)
	s.hooks.Fire(ctx, hooks.SessionEnd, map[string]any{
		"reason": "completed", "duration_ms": time.Since(s.started).Milliseconds(),
		"is_background_agent": false, "final_status": "completed",
	})
	s.tr.end()
	if runErr != nil {
		return fmt.Errorf("cursor-mock: %w", runErr)
	}
	if s.result != nil {
		s.forward(s.result)
	}
	return nil
}

// common is what every hook payload carries now: the transcript path only once
// the conversation has a transcript.
func (s *session) common() hooks.Common {
	c := hooks.Common{SessionID: s.id, Dir: s.cfg.Dir, Version: s.cfg.Version}
	if s.tr.exists() {
		c.TranscriptPath = s.tr.path
	}
	return c
}

// forward prints one stream-json line.
func (s *session) forward(line []byte) { fmt.Fprintf(s.cfg.Stdout, "%s\n", line) }

// pending is a tool call the script asked for.
type pending struct {
	call toolexec.Call
	id   string
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func jsonLine(v any) []byte { b, _ := json.Marshal(v); return b }
