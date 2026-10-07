package replay

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// transcriptWrites are the Write calls the conversations under root left in
// their transcripts, one object per call: the input as Cursor records it
// (an absolute path and the file's "contents"). Several conversations (the
// main session and its sub-agents) are one set, in their own order of file
// name, which the two sides do not share, so the set is sorted by the caller.
func transcriptWrites(root string) ([]map[string]any, error) {
	var out []map[string]any
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		records, err := readJSONL(path)
		if err != nil {
			return err
		}
		for _, r := range records {
			msg, _ := r["message"].(map[string]any)
			blocks, _ := msg["content"].([]any)
			for _, b := range blocks {
				blk, _ := b.(map[string]any)
				if in, ok := blk["input"].(map[string]any); ok && blk["type"] == "tool_use" && blk["name"] == "Write" {
					out = append(out, map[string]any{"transcriptWrite": in})
				}
			}
		}
		return nil
	})
	return out, err
}

// sortedLines is lines in a fixed order.
func sortedLines(lines []string) []string {
	sort.Strings(lines)
	return lines
}
