package replay

import "strings"

// harnessSkills is where cursor-agent keeps the skills it bundles, in the home it
// runs under: files of the harness's own, which it puts there before the run and
// which a model may read (recorded: runs/schedule-wakeup-ask reads the loop skill).
const harnessSkills = "<TMP>/home/.cursor/skills-cursor/"

// homeFilePrefix names a Setup entry that is a file the harness had in its home:
// the rest of the key is its path under the home.
const homeFilePrefix = "home:"

// harnessFiles are the harness's own files a run read: what a completed Read
// frame of a path under the harness's skills shows as its content, by the path
// under the home. The mock has no such files of its own; the replay lays them out
// from what the recording shows, so the read it plays answers the same.
func harnessFiles(stream []map[string]any) map[string]string {
	files := map[string]string{}
	for _, f := range stream {
		if f["type"] != "tool_call" || f["subtype"] != "completed" {
			continue
		}
		call, _ := f["tool_call"].(map[string]any)
		read, _ := call["readToolCall"].(map[string]any)
		args, _ := read["args"].(map[string]any)
		path, _ := args["path"].(string)
		result, _ := read["result"].(map[string]any)
		success, _ := result["success"].(map[string]any)
		content, ok := success["content"].(string)
		if rel, under := strings.CutPrefix(path, "<TMP>/home/"); ok && under && strings.HasPrefix(path, harnessSkills) {
			files[homeFilePrefix+rel] = content
		}
	}
	return files
}
