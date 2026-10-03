package events

import "io"

// Abort ends the stream: the turn was aborted, so nothing more of it is
// printed (no turn.completed). Recorded: runs/manual-compaction-auto-blocked.
func (s *Stream) Abort() { s.out = io.Discard }
