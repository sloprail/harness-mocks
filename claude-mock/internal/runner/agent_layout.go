package runner

import "github.com/sloprail/harness-mocks/internal/subagents"

// claudeSubagentLayout is where Claude Code keeps a sub-agent's transcript:
// <session transcript without .jsonl>/subagents/agent-<id>.jsonl, in the one
// directory of the session, for a nested sub-agent too (and for the summarizer
// of a manual compaction, whose file is never written).
//
// sr:provides subagent-transcripts/claude
// sr:provides nested-subagents/claude
var claudeSubagentLayout = subagents.Layout{SessionExt: claudeLayout.Ext, Dir: "subagents", Prefix: "agent-", Ext: claudeLayout.Ext, SidecarExt: ".meta.json"}
