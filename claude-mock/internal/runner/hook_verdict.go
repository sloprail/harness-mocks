package runner

import "encoding/json"

// hookBlocked reads a hook command's raw result itself: exit code 2 blocks,
// and so does a JSON body whose decision is "block".
func hookBlocked(exitCode int, stdout []byte) (bool, string) {
	if exitCode == 2 {
		return true, string(stdout)
	}
	var out struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if json.Unmarshal(stdout, &out) == nil && out.Decision == "block" {
		return true, out.Reason
	}
	return false, ""
}
