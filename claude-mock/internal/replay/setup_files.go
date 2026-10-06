package replay

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
)

// A sample may have been recorded when a file of the setup read otherwise than it does now: the model's
// Read of that file shows what it held. The replay of that sample installs the file as the Read saw it, so
// that the mock reads (and runs) what the recording did.

var numbered = regexp.MustCompile(`^\s*\d+\t`)

// readSetupFiles are the setup files (hook.sh) the model read whole in the sample, as it read them, by
// name: the content of each Read result, its line numbers removed. A file read in part is not.
func readSetupFiles(sample string) map[string]string {
	var all [][]map[string]any
	files, _ := filepath.Glob(filepath.Join(sample, "transcript", "*.jsonl"))
	subs, _ := filepath.Glob(filepath.Join(sample, "transcript", "*", "subagents", "*.jsonl"))
	for _, f := range append(files, subs...) {
		if recs, err := readJSONL(f); err == nil {
			all = append(all, recs)
		}
	}
	reads := map[string]string{} // call id -> file name, of a whole read of a setup file
	out := map[string]string{}
	for _, recs := range all {
		for _, rec := range recs {
			msg, _ := rec["message"].(map[string]any)
			blocks, _ := msg["content"].([]any)
			for _, b := range blocks {
				block, _ := b.(map[string]any)
				switch block["type"] {
				case "tool_use":
					in, _ := block["input"].(map[string]any)
					path, _ := in["file_path"].(string)
					id, _ := block["id"].(string)
					if name, ok := strings.CutPrefix(path, "<RUN>/"); block["name"] == "Read" && ok && name == "hook.sh" && in["offset"] == nil && in["limit"] == nil {
						reads[id] = name
					}
				case "tool_result":
					id, _ := block["tool_use_id"].(string)
					text, _ := block["content"].(string)
					if name, ok := reads[id]; ok && block["is_error"] != true {
						out[name] = unnumber(text)
					}
				}
			}
		}
	}
	return out
}

// unnumber is a Read result's text without the line numbers it prefixes each line with.
func unnumber(text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = numbered.ReplaceAllString(l, "")
	}
	return strings.TrimSuffix(strings.Join(lines, "\n"), "\n") + "\n"
}

func filesJSON(files map[string]string) string {
	if len(files) == 0 {
		return ""
	}
	b, _ := json.Marshal(files)
	return string(b)
}

// changedSetupFiles are the setup files whose content in the sample differs from the setup's now.
func changedSetupFiles(setup, sample string) map[string]string {
	out := map[string]string{}
	for name, content := range readSetupFiles(sample) {
		if content != readFile(filepath.Join(setup, name)) {
			out[name] = content
		}
	}
	return out
}
