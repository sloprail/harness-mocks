package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEveryPayloadCarriesTheCommonSchemaFieldsOfTheRecording: recorded
// (runs/file-tools), every hook payload carries cursor_version, user_email
// (null: the login's email is never kept), generation_id, model and a
// conversation_id that is the same in every payload of the run; the mock's
// payloads carry the same fields, the same conversation_id throughout.
// sr:proves hook-common-payload/cursor
func TestEveryPayloadCarriesTheCommonSchemaFieldsOfTheRecording(t *testing.T) {
	_, want, _, _ := recording(t, "file-tools")
	require.NotEmpty(t, want.hooks)
	got, _ := replay(t, "file-tools")
	for name, payloads := range map[string][]map[string]any{"recorded": recordedRaw(t, "file-tools"), "mock": got.raw} {
		require.NotEmpty(t, payloads, name)
		var conversation string
		for _, p := range payloads {
			ev, _ := p["hook_event_name"].(string)
			for _, k := range []string{"cursor_version", "generation_id", "model", "conversation_id"} {
				v, ok := p[k].(string)
				assert.True(t, ok, "%s: %s carries %s", name, ev, k)
				if k != "model" { // a model of "" is what a sub-agent's call names
					assert.NotEmpty(t, v, "%s: %s's %s", name, ev, k)
				}
			}
			assert.Contains(t, p, "user_email", "%s: %s", name, ev)
			assert.Nil(t, p["user_email"], "%s: %s: never kept", name, ev)
			if conversation == "" {
				conversation = p["conversation_id"].(string)
			}
			assert.Equal(t, conversation, p["conversation_id"], "%s: %s: the conversation is the same throughout", name, ev)
		}
	}
}

// frameFields reads what a stream's assistant and tool_call frames carry.
func frameFields(t *testing.T, stream string) (frames []map[string]any) {
	t.Helper()
	for _, l := range strings.Split(stream, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil && (f["type"] == "assistant" || f["type"] == "tool_call") {
			frames = append(frames, f)
		}
	}
	return frames
}

// TestAssistantAndToolCallFramesCarryTheirModelCallAndTheClock: recorded
// (runs/file-tools), every tool_call frame, and the assistant frame a call
// brings out, carries model_call_id, timestamp_ms and the run's session_id, a
// call's started and completed frames carry the same model_call_id, and the
// assistant frame at the end of the turn carries neither field. The mock's
// stream does the same.
// sr:proves noninteractive-run/cursor
func TestAssistantAndToolCallFramesCarryTheirModelCallAndTheClock(t *testing.T) {
	got, _ := replay(t, "file-tools")
	recorded, err := filepath.Glob(filepath.Join(newestSample(t, "file-tools"), "stream.jsonl"))
	require.NoError(t, err)
	b := readFileOrFail(t, recorded[0])
	for name, stream := range map[string]string{"recorded": b, "mock": got.stdout} {
		frames := frameFields(t, stream)
		require.NotEmpty(t, frames, name)
		var session string
		for _, f := range frames {
			if session == "" {
				session, _ = f["session_id"].(string)
			}
			assert.Equal(t, session, f["session_id"], name)
		}
		body := frames
		if last := len(frames) - 1; frames[last]["type"] == "assistant" { // the text that ends the turn
			assert.NotContains(t, frames[last], "model_call_id", name+": the end of the turn")
			assert.NotContains(t, frames[last], "timestamp_ms", name)
			body = frames[:last]
		} else {
			require.Equal(t, "mock", name, "recorded: the turn ends with the assistant's text")
		}
		ids := map[string]string{}
		for _, f := range body {
			id, ok := f["model_call_id"].(string)
			require.True(t, ok, "%s: a %v frame carries model_call_id", name, f["type"])
			assert.NotEmpty(t, id, name)
			_, hasClock := f["timestamp_ms"].(float64)
			assert.True(t, hasClock, "%s: a %v frame carries timestamp_ms", name, f["type"])
			if f["type"] == "tool_call" {
				call, _ := f["call_id"].(string)
				if f["subtype"] == "started" {
					ids[call] = id
				} else {
					assert.Equal(t, ids[call], id, "%s: a call's frames name the same model call", name)
				}
			}
		}
	}
}
