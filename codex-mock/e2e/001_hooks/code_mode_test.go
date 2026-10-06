package e2e

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A tool call of the model's script that a hook rejects makes the call throw the hook's reason: a
// PreToolUse block rejects it before the tool runs, a PostToolUse block (exit 2 or a decision) after,
// and the agent is told the script failed with that reason as its error, framed as the harness frames
// it ("Script failed", the time, "Output:", "Script error:"), whole, only the time masked
// (recorded: runs/pretool-decisions, runs/stops, runs/posttool-block; hooks#posttooluse, Tool calls
// from code mode).
// sr:proves hook-exit-code-semantics/codex
// sr:proves pretooluse-refusal/codex
func TestAHookRejectingAToolCallMakesItThrowTheReasonAsAScriptError(t *testing.T) {
	wall := regexp.MustCompile(`Wall time [0-9.]+ seconds`)
	failures := func(rollout string) (out []string) {
		for _, o := range toolOutputs(t, rollout) {
			if regexp.MustCompile(`^Script failed`).MatchString(o) {
				out = append(out, wall.ReplaceAllString(o, "Wall time <T> seconds"))
			}
		}
		return
	}
	for run, want := range map[string]string{
		"posttool-block": "Script failed\nWall time <T> seconds\nOutput:\nScript error:\nPOST-FEEDBACK-MSG",
	} {
		rec := loadRecording(t, run)
		recorded := failures(recordedRollout(t, rec))
		assert.NotEmpty(t, recorded, run)
		for _, f := range recorded {
			assert.Equal(t, want, f, run+": recorded")
		}
		for _, f := range failures(replay(t, rec).rollout(t)) {
			assert.Equal(t, want, f, run+": the mock's")
		}
		assert.Equal(t, len(recorded), len(failures(replay(t, rec).rollout(t))), run)
	}
	for _, run := range []string{"stops", "pretool-decisions"} {
		rec := loadRecording(t, run)
		recorded := failures(recordedRollout(t, rec))
		assert.NotEmpty(t, recorded, run)
		for _, f := range recorded {
			assert.Regexp(t, `^Script failed\nWall time <T> seconds\nOutput:\nScript error:\nCommand blocked by PreToolUse hook: .+\. Command: echo [A-Z0-9]+$`, f, run+": recorded")
		}
		assert.Equal(t, recorded, failures(replay(t, rec).rollout(t)), run+": the mock tells the same")
	}
}
