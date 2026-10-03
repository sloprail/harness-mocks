package session

// TurnAborted records that the turn was interrupted.
func (s *File) TurnAborted(turnID string) {
	s.append("event_msg", map[string]any{"type": "turn_aborted", "turn_id": turnID, "reason": "interrupted"})
}

// Compacted records that the session was compacted: the record Codex leaves
// where the conversation so far was replaced.
func (s *File) Compacted() {
	s.append("compacted", map[string]any{"message": "", "replacement_history": []any{}})
}
