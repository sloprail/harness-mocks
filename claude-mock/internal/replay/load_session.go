package replay

// sessionOf is the session id the stream names.
func sessionOf(stream []map[string]any) string {
	for _, f := range stream { // a hook frame ahead of it may name another (a resume's start hook)
		if id, _ := f["session_id"].(string); id != "" && f["type"] == "system" && f["subtype"] == "init" {
			return id
		}
	}
	for _, f := range stream {
		if id, _ := f["session_id"].(string); id != "" {
			return id
		}
	}
	return ""
}
