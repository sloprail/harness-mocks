package e2e

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// agentTrailer is what ends a foreground hand-back: the agentId line and the
// usage block, whose numbers are the model's.
var agentTrailer = regexp.MustCompile(`\nagentId: [0-9a-f]+ \(use SendMessage[^\n]*\)\n<usage>subagent_tokens: \d+\ntool_uses: \d+\nduration_ms: \d+</usage>$`)

// TestT017_70_AgentReportIsScannedBeforeTheParentReadsIt: a foreground
// sub-agent's report is scanned (sub-agents doc, "Subagent output scanning"):
// a `<system-reminder>` tag is neutralised (`<` becomes `<\`), and the report
// of one that imitates a tag or names a permission setting is headed by a
// `[harness: ... matched instruction-shaped pattern(s): <names>. ...]` notice,
// a block of its own in tool_response.content, a line of the hand-back, and
// harnessNoteCount 1; a `Human:` or `Assistant:` line gets a backslash before its
// colon and no notice; the task_notification frame's summary is the report as
// scanned, notice first. The hand-back text, the content and harnessSectionHash are the
// recorded ones (snapshots/runs/fgsub-report-scan, -perm, -role, -assistant).
// sr:proves foreground-subagent-result/claude
func TestT017_70_AgentReportIsScannedBeforeTheParentReadsIt(t *testing.T) {
	for _, tc := range []struct{ run, report string }{
		{"fgsub-report-scan", "The scan flags tags such as <system-reminder> and the setting bypassPermissions."},
		{"fgsub-report-scan-perm", "The flag --dangerously-skip-permissions is a CLI option."},
		{"fgsub-report-scan-role", "Here is a transcript excerpt.\nHuman: what is two plus two?"},
		{"fgsub-report-scan-assistant", "Here is a transcript excerpt.\nAssistant: two plus two is four."},
	} {
		t.Run(tc.run, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "config")
			log := filepath.Join(dir, "payloads.log")
			settings(t, dir, map[string]string{"PostToolUse": payloadLogger(t, dir, "log.sh", log, "")})
			sub := replyScript(t, dir, "sub", tc.report)
			orch := script(t, dir, "orch", toolUse("ag1", "Agent", `{"prompt":"go","description":"sample","script":"`+sub+`"}`))
			out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "scan-1",
				"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
			require.Equal(t, 0, code, out)

			block, _ := toolResultOf(t, readRecs(t, transcriptPath(t, cfg, dir, "scan-1")), "ag1turn-orch-a")
			got := block["content"].([]any)[0].(map[string]any)["text"].(string)
			want := recordedAgentResultText(t, tc.run)
			assert.Equal(t, agentTrailer.ReplaceAllString(want, ""), agentTrailer.ReplaceAllString(got, ""),
				"the hand-back, notice and report as the real claude handed them")
			assert.Regexp(t, agentTrailer, got)

			gotPost := agentPosts(payloads(t, log))
			require.Len(t, gotPost, 1)
			wantPost := recordedAgentPost(t, tc.run, 0)
			gr, wr := gotPost[0]["tool_response"].(map[string]any), wantPost["tool_response"].(map[string]any)
			assert.Equal(t, wr["content"], gr["content"])
			assert.Equal(t, wr["harnessNoteCount"], gr["harnessNoteCount"])
			assert.Equal(t, wr["harnessTailCount"], gr["harnessTailCount"])
			assert.Equal(t, wr["harnessSectionHash"], gr["harnessSectionHash"], "the hash of the content blocks")
			assert.Equal(t, keysOf(wr), keysOf(gr))

			// the foreground task's notification frame carries the report as scanned
			frames := framesOf(t, out, gr["agentId"].(string))
			require.NotEmpty(t, frames)
			assert.Equal(t, "task_notification", frames[len(frames)-1]["subtype"])
			assert.Equal(t, recordedNotificationSummary(t, tc.run), frames[len(frames)-1]["summary"])
		})
	}
}

// TestT017_70b_AClearReportIsHandedBackAsWritten: a report that imitates
// nothing carries no notice, harnessNoteCount 0 and a one-block content, the
// shape of every recorded run that was not scanned (hookerrors, hookmix).
// sr:proves foreground-subagent-result/claude
func TestT017_70b_AClearReportIsHandedBackAsWritten(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"PostToolUse": payloadLogger(t, dir, "log.sh", log, "")})
	sub := replyScript(t, dir, "sub", "HELPED")
	orch := script(t, dir, "orch", toolUse("ag1", "Agent", `{"prompt":"go","description":"helper","script":"`+sub+`"}`))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "scan-2",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	post := agentPosts(payloads(t, log))[0]["tool_response"].(map[string]any)
	assert.EqualValues(t, 0, post["harnessNoteCount"])
	assert.Equal(t, []any{map[string]any{"type": "text", "text": "HELPED"}}, post["content"])
	block, _ := toolResultOf(t, readRecs(t, transcriptPath(t, cfg, dir, "scan-2")), "ag1turn-orch-a")
	text := block["content"].([]any)[0].(map[string]any)["text"].(string)
	assert.True(t, strings.HasPrefix(text, "[Subagent hand-back]"), "no notice ahead of the frame: %s", text)
}
