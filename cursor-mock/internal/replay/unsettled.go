package replay

// unsettled marks what Cursor does not settle: whether the first
// beforeShellExecution of a run already names the transcript. It does in some
// captures and carries null in others (recorded: runs/tool-failure and
// runs/symlinked-cwd name it; runs/shell-exit-status and runs/hook-json-nonzero
// do not), and the mock names it, one of the two (spec/capabilities/
// session-transcript-file.yaml). That one payload's transcript_path, and what a
// hook script logged of it for that event, is not compared: it is set to a
// marker on both sides, the key kept.
func unsettled(payloads []map[string]any) {
	in := false // the lines of the first beforeShellExecution's hook run
	done := false
	for _, p := range payloads {
		if done {
			return
		}
		if e, _ := p["hook_event_name"].(string); e != "" {
			if in {
				done = true
				continue
			}
			if e == "beforeShellExecution" {
				in = true
				if _, ok := p["transcript_path"]; ok {
					p["transcript_path"] = "<unsettled>"
				}
			}
			continue
		}
		if !in {
			continue
		}
		if r, ok := p["hook_result"].(map[string]any); ok { // what a hook script logged about its payload's path
			for _, k := range []string{"transcript_path_set", "transcript_exists"} {
				if _, ok := r[k]; ok {
					r[k] = "<unsettled>"
				}
			}
		}
		if env, ok := p["hook_env"].(map[string]any); ok { // and about its environment: the key is as unsettled as the value
			env["CURSOR_TRANSCRIPT_PATH"] = "<unsettled>"
		}
	}
}
