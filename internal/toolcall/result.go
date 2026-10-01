package toolcall

import "strings"

// ResultText is what the agent is given for a tool's output: the output
// itself, or, when it is empty or only whitespace, a placeholder that names the
// tool, so no tool result reaches the agent with nothing in it.
//
// sr:capability empty-tool-result-placeholder
func ResultText(tool, output string) string {
	if strings.TrimSpace(output) == "" {
		return "(" + tool + " completed with no output)"
	}
	return output
}
