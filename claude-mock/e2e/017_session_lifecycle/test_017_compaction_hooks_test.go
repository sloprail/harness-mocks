package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// compactSettings configures one command hook per event, each with its own matcher.
func compactSettings(t *testing.T, dir string, hooks map[string][2]string) {
	t.Helper()
	h := map[string]any{}
	for ev, matcherAndCmd := range hooks {
		h[ev] = []any{map[string]any{"matcher": matcherAndCmd[0], "hooks": []any{map[string]any{"type": "command", "command": matcherAndCmd[1]}}}}
	}
	b, err := json.Marshal(map[string]any{"hooks": h})
	require.NoError(t, err)
	write(t, filepath.Join(dir, ".claude", "settings.json"), string(b), 0o644)
}

func compactRun(t *testing.T, dir, cfg, id, trigger string) {
	t.Helper()
	sc := script(t, dir, "s", `{"type":"compact","summary":"x @MARK@","trigger":"`+trigger+`"}`)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", id,
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
}

// TestT017_07f_PreCompactJSONBlockStopsTheCompaction: a PreCompact hook can also
// block by returning JSON with "decision": "block" (hooks#precompact). Nothing is
// written, and PostCompact does not fire, because nothing was compacted.
// sr:proves manual-compaction/claude
func TestT017_07f_PreCompactJSONBlockStopsTheCompaction(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "post.log")
	block := write(t, filepath.Join(dir, "block.sh"), "#!/bin/sh\ncat >/dev/null\necho '{\"decision\":\"block\",\"reason\":\"not now\"}'\n", 0o755)
	post := payloadLogger(t, dir, "post.sh", log, "")
	compactSettings(t, dir, map[string][2]string{"PreCompact": {"*", block}, "PostCompact": {"*", post}})
	compactRun(t, dir, cfg, "cmp-f", "manual")
	raw, err := os.ReadFile(transcriptPath(t, cfg, dir, "cmp-f"))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "compact_boundary")
	assert.NotContains(t, string(raw), "isCompactSummary")
	_, err = os.Stat(log)
	assert.True(t, os.IsNotExist(err), "PostCompact fires only when the compaction happens")
}

// TestT017_07g_PreCompactCannotStopWithContinueFalse: Claude Code discards a
// PreCompact hook's `continue` and `systemMessage` fields (hooks#precompact), so
// they neither block nor change the compaction.
// sr:proves manual-compaction/claude
func TestT017_07g_PreCompactCannotStopWithContinueFalse(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	pre := write(t, filepath.Join(dir, "pre.sh"), "#!/bin/sh\ncat >/dev/null\necho '{\"continue\":false,\"systemMessage\":\"ignored\"}'\n", 0o755)
	compactSettings(t, dir, map[string][2]string{"PreCompact": {"*", pre}})
	compactRun(t, dir, cfg, "cmp-g", "manual")
	raw, err := os.ReadFile(transcriptPath(t, cfg, dir, "cmp-g"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "compact_boundary", "the compaction happens")
}

// TestT017_07i_ManualCompactionBlockShowsTheMessage: exit 2 from PreCompact blocks
// the compaction, and for a manual /compact its stderr message is shown to the
// user (hooks#precompact). An automatic compaction blocked the same way writes
// nothing either.
// sr:proves manual-compaction/claude
func TestT017_07i_ManualCompactionBlockShowsTheMessage(t *testing.T) {
	for _, trigger := range []string{"manual", "auto"} {
		t.Run(trigger, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "config")
			block := write(t, filepath.Join(dir, "block.sh"), "#!/bin/sh\ncat >/dev/null\necho 'compaction not allowed now' 1>&2\nexit 2\n", 0o755)
			compactSettings(t, dir, map[string][2]string{"PreCompact": {"*", block}})
			sc := script(t, dir, "s", `{"type":"compact","summary":"x @MARK@","trigger":"`+trigger+`"}`)
			out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "cmp-i",
				"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
			require.Equal(t, 0, code, out)
			if trigger == "manual" {
				assert.Contains(t, out, "compaction not allowed now", "the block's message is shown to the user")
			} else {
				assert.NotContains(t, out, "compaction not allowed now")
			}
			raw, _ := os.ReadFile(transcriptPath(t, cfg, dir, "cmp-i"))
			assert.NotContains(t, string(raw), "compact_boundary")
		})
	}
}

// TestT017_07j_CompactionStreamsItsFrames: a compaction streams, as the recorded
// manual /compact run did, system/status "compacting", then status with
// compact_result "success", a system/init, the system/compact_boundary frame (the
// boundary's metadata in snake_case, the transcript boundary's logical parent and
// kept records), the summary, and for a manual compaction the command's output.
// sr:proves manual-compaction/claude
// sr:proves compaction-transcript-continuity/claude
func TestT017_07j_CompactionStreamsItsFrames(t *testing.T) {
	for _, trigger := range []string{"manual", "auto"} {
		t.Run(trigger, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "config")
			sc := script(t, dir, "s", toolUse("b1", "Bash", `{"command":"true"}`),
				`{"type":"compact","summary":"the summary @MARK@","trigger":"`+trigger+`","pre_tokens":900,"post_tokens":100}`)
			out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "cmp-j",
				"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
			require.Equal(t, 0, code, out)
			var frames []map[string]any
			for _, l := range strings.Split(out, "\n") {
				var f map[string]any
				if json.Unmarshal([]byte(l), &f) == nil && f != nil {
					frames = append(frames, f)
				}
			}
			var at = func(typ, subtype string) int {
				for i, f := range frames {
					if f["type"] == typ && f["subtype"] == subtype && (subtype != "status" || f["status"] == "compacting") {
						return i
					}
				}
				return -1
			}
			compacting, boundary := at("system", "status"), at("system", "compact_boundary")
			require.GreaterOrEqual(t, compacting, 0, "status compacting")
			require.Greater(t, boundary, compacting)
			ended := frames[compacting+1]
			assert.Equal(t, "status", ended["subtype"])
			assert.Nil(t, ended["status"])
			assert.Equal(t, "success", ended["compact_result"])
			initFrame := frames[compacting+2]
			assert.Equal(t, "init", initFrame["subtype"])
			assert.NotEmpty(t, initFrame["cwd"])
			assert.Equal(t, boundary, compacting+3, "status, status, init, then the boundary")
			f := frames[boundary]
			assert.Equal(t, "cmp-j", f["session_id"])
			meta := f["compact_metadata"].(map[string]any)
			assert.Equal(t, trigger, meta["trigger"])
			assert.EqualValues(t, 900, meta["pre_tokens"])
			assert.EqualValues(t, 100, meta["post_tokens"])
			assert.EqualValues(t, 800, meta["cumulative_dropped_tokens"])
			recs := readRecs(t, transcriptPath(t, cfg, dir, "cmp-j"))
			for _, r := range recs {
				if r.Subtype != "compact_boundary" {
					continue
				}
				assert.Equal(t, r.LogicalParentUUID, f["logical_parent_uuid"], "the frame names the transcript boundary's logical parent")
				var full map[string]any
				require.NoError(t, json.Unmarshal([]byte(r.Raw), &full))
				kept := full["compactMetadata"].(map[string]any)["preservedMessages"].(map[string]any)["uuids"]
				assert.Equal(t, kept, meta["preserved_messages"].(map[string]any)["uuids"])
				assert.Contains(t, meta["preserved_segment"], "tail_uuid")
			}
			summary, command := false, false
			for _, fr := range frames[boundary+1:] {
				if fr["isCompactSummary"] == true {
					summary = true
				}
				if msg, _ := fr["message"].(map[string]any); msg != nil {
					if c, _ := msg["content"].(string); strings.HasPrefix(c, "<local-command-stdout>Compacted") {
						command = true
					}
				}
			}
			assert.True(t, summary, "the summary follows the boundary")
			assert.Equal(t, trigger == "manual", command, "only a manual compaction is the /compact command, whose output streams")
		})
	}
}

// TestT017_07h_CompactHookMatchersSelectTheTrigger: the matcher of PreCompact and
// PostCompact is the trigger, `manual` for /compact and `auto` for auto-compact
// (hooks#precompact, hooks#postcompact).
// sr:proves manual-compaction/claude
func TestT017_07h_CompactHookMatchersSelectTheTrigger(t *testing.T) {
	for _, tc := range []struct {
		matcher, trigger string
		fires            bool
	}{
		{"manual", "manual", true}, {"auto", "manual", false},
		{"auto", "auto", true}, {"manual", "auto", false},
	} {
		t.Run(tc.matcher+"-matcher-"+tc.trigger+"-trigger", func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "config")
			log := filepath.Join(dir, "payloads.log")
			h := payloadLogger(t, dir, "log.sh", log, "")
			compactSettings(t, dir, map[string][2]string{"PreCompact": {tc.matcher, h}, "PostCompact": {tc.matcher, h}})
			compactRun(t, dir, cfg, "cmp-h", tc.trigger)
			if !tc.fires {
				_, err := os.Stat(log)
				assert.True(t, os.IsNotExist(err), "the %s matcher does not select a %s compaction", tc.matcher, tc.trigger)
				return
			}
			var events []any
			for _, p := range payloads(t, log) {
				assert.Equal(t, tc.trigger, p["trigger"])
				events = append(events, p["hook_event_name"])
			}
			assert.Equal(t, []any{"PreCompact", "PostCompact"}, events)
		})
	}
}
