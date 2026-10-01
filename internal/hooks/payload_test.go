package hooks

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCommonFields(t *testing.T) {
	session := Common{TranscriptPath: "/s/t.jsonl", Cwd: "/work"}
	sub := Agent{ID: "a1", Type: "general-purpose"}
	for _, tc := range []struct {
		name   string
		event  Common
		inside Agent
		want   Common
	}{
		{"main thread names no agent", Common{}, Agent{}, Common{TranscriptPath: "/s/t.jsonl", Cwd: "/work"}},
		{"inside a sub-agent it is named", Common{}, sub, Common{TranscriptPath: "/s/t.jsonl", Cwd: "/work", Agent: sub}},
		{"the event's own path and cwd stay", Common{TranscriptPath: "/o.jsonl", Cwd: "/iso"}, Agent{}, Common{TranscriptPath: "/o.jsonl", Cwd: "/iso"}},
		{"an event naming another agent keeps it", Common{Agent: Agent{ID: "a2", Type: "x"}}, sub, Common{TranscriptPath: "/s/t.jsonl", Cwd: "/work", Agent: Agent{ID: "a2", Type: "x"}}},
		{"the event naming this agent gets its type", Common{Agent: Agent{ID: "a1"}}, sub, Common{TranscriptPath: "/s/t.jsonl", Cwd: "/work", Agent: sub}},
	} {
		if got := CommonFields(tc.event, session, tc.inside); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestNewPostTool(t *testing.T) {
	in, resp := json.RawMessage(`{"command":"ls"}`), json.RawMessage(`{"stdout":"x"}`)
	got := NewPostTool(in, resp, 1500*time.Microsecond)
	if string(got.Input) != string(in) || string(got.Response) != string(resp) || got.DurationMs != 1 {
		t.Errorf("got %+v", got)
	}
	if got := NewPostTool(in, resp, -time.Second); got.DurationMs != 0 {
		t.Errorf("a negative duration is %d, want 0", got.DurationMs)
	}
}

func TestNewStop(t *testing.T) {
	if s := NewStop("done", 0); s.LastMessage != "done" || s.Continuing {
		t.Errorf("first stop: %+v", s)
	}
	if s := NewStop("done", 3); !s.Continuing {
		t.Errorf("after blocks: %+v", s)
	}
}
