package events

// FileChange is how Codex shows a patch's files: an item with each file's path
// and the kind of change (add, update, delete), started and completed with the
// patch's status (recorded: runs/file-tools). The pair is reported together,
// as the patch is applied at once.
func (s *Stream) FileChange(changes []map[string]string, status string) {
	id := s.newID()
	for _, st := range []string{"in_progress", status} {
		typ := "item.started"
		if st != "in_progress" {
			typ = "item.completed"
		}
		s.emit(map[string]any{"type": typ, "item": map[string]any{
			"id": id, "type": "file_change", "changes": changes, "status": st}})
	}
}
