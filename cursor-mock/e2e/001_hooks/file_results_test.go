package e2e

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// resultKeys are, for each completed tool call of a stream in order, the call's
// kind, the outcome and the keys the outcome carries.
func resultKeys(frames []map[string]any) (out []string) {
	for _, f := range frames {
		tc, _ := f["tool_call"].(map[string]any)
		if f["type"] != "tool_call" || f["subtype"] != "completed" {
			continue
		}
		for kind, v := range tc {
			body, _ := v.(map[string]any)
			res, _ := body["result"].(map[string]any)
			for outcome, o := range res {
				keys := []string{}
				if m, ok := o.(map[string]any); ok {
					for k := range m {
						keys = append(keys, k)
					}
				}
				sort.Strings(keys)
				out = append(out, kind+" "+outcome+" "+join(keys))
			}
		}
	}
	return out
}

func join(s []string) (j string) {
	for i, x := range s {
		if i > 0 {
			j += ","
		}
		j += x
	}
	return j
}

// TestTheFileToolsResultFramesCarryWhatTheRecordingsShow: recorded, a Write's
// completed frame carries its path, the lines added and removed, the diff, the
// content after (and, for one that replaces a file, before) and a message; a
// Read's, the content, its size and line count and the range read; a Read of a
// missing file, an error with a message only (runs/file-tools,
// runs/tool-failure). The mock's frames carry the same keys.
// sr:proves task-stream-frames/cursor
func TestTheFileToolsResultFramesCarryWhatTheRecordingsShow(t *testing.T) {
	var recorded []string
	for _, run := range []string{"file-tools", "tool-failure"} {
		for _, k := range resultKeys(recordedStream(t, run)) {
			if len(k) > 0 && k[:4] != "shel" {
				recorded = append(recorded, k)
			}
		}
	}
	require.Contains(t, recorded, "editToolCall success afterFullFileContent,diffString,linesAdded,linesRemoved,message,path")
	require.Contains(t, recorded, "editToolCall success afterFullFileContent,beforeFullFileContent,diffString,linesAdded,linesRemoved,message,path")
	require.Contains(t, recorded, "readToolCall success content,exceededLimit,fileSize,isEmpty,path,readRange,relatedCursorRulePaths,relatedCursorRules,totalLines")
	require.Contains(t, recorded, "readToolCall error errorMessage")

	r := runTools(t, `{"version":1,"hooks":{}}`, nil, map[string]string{"old.txt": "hi\n"},
		map[string]any{"name": "Write", "input": map[string]any{"file_path": "new.txt", "content": "hi\n"}},
		map[string]any{"name": "Edit", "input": map[string]any{"file_path": "old.txt", "old_string": "hi", "new_string": "bye"}},
		map[string]any{"name": "Read", "input": map[string]any{"file_path": "old.txt"}},
		map[string]any{"name": "Read", "input": map[string]any{"file_path": "missing.txt"}})
	require.Equal(t, []string{
		"editToolCall success afterFullFileContent,diffString,linesAdded,linesRemoved,message,path",
		"editToolCall success afterFullFileContent,beforeFullFileContent,diffString,linesAdded,linesRemoved,message,path",
		"readToolCall success content,exceededLimit,fileSize,isEmpty,path,readRange,relatedCursorRulePaths,relatedCursorRules,totalLines",
		"readToolCall error errorMessage",
	}, resultKeys(r.frames))
}
