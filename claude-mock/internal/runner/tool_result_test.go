package runner

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
)

// A tool result with no content is given to the agent as a placeholder that
// names the tool, in the stream frame and in the transcript record alike, for
// whichever tool it was (claude 2.1.285, recorded: snapshots/runs/bashfail:
// "(Bash completed with no output)"); a result with content, spaces and all, is
// given as it is.
// sr:proves empty-tool-result-placeholder/claude
func TestEmitToolResult_PlaceholderNamesTheTool(t *testing.T) {
	for _, tc := range []struct{ tool, output, want string }{
		{"Bash", "", "(Bash completed with no output)"},
		{"Glob", "", "(Glob completed with no output)"},
		{"Read", " \n\t", "(Read completed with no output)"},
		{"Write", "", "(Write completed with no output)"},
		{"Bash", "  padded  ", "  padded  "},
	} {
		var buf bytes.Buffer
		lw := &lockedWriter{w: &buf}
		cfg := Config{Out: lw, stream: lw}
		var streamed string
		recs := recordsOf(t, func(tr *transcript) {
			require.NoError(t, emitToolResult(cfg, pendingToolUse{ToolUseID: "tu1", ToolName: tc.tool}, toolexec.Result{Output: tc.output}, tr))
		})
		var frame struct {
			Message struct {
				Content []struct {
					Content string `json:"content"`
				} `json:"content"`
			} `json:"message"`
		}
		require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &frame))
		streamed = frame.Message.Content[0].Content
		assert.Equal(t, tc.want, streamed, "%s in the stream", tc.tool)
		require.Len(t, recs, 1)
		blocks := recs[0]["message"].(map[string]any)["content"].([]any)
		assert.Equal(t, tc.want, blocks[0].(map[string]any)["content"], "%s in the transcript", tc.tool)
	}
}
