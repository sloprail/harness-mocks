package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_47_HTTPHookAnswers: a hook that is an HTTP endpoint gets the event's
// payload as the body of a POST, with Content-Type application/json, and its
// answer is read as the docs say (HTTP response handling; snapshots/runs/http-hook
// for an endpoint that cannot be reached): an empty 2xx is a success, a 2xx JSON
// object decides like a command's JSON (here a refusal), and any other answer, a
// non-2xx status, a plain-text body, an unreachable endpoint, is a non-blocking
// error recorded as such while the call goes ahead. No status alone blocks.
// sr:proves http-hooks/claude
// sr:proves hook-output-transcript-records/claude
func TestT017_47_HTTPHookAnswers(t *testing.T) {
	closed := func() string {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := l.Addr().String()
		l.Close()
		return "http://" + addr + "/hook"
	}()
	for _, tc := range []struct {
		name   string
		status int
		body   string
		url    string // overrides the server's
		want   string // the call's result
		errMsg string // the non-blocking error recorded, "" for none
	}{
		{"empty 2xx", 200, "", "", "RAN", ""},
		{"2xx json deny", 200, `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"HTTP-DENY"}}`, "", "HTTP-DENY", ""},
		{"non-2xx with a deny body", 500, `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"HTTP-DENY"}}`, "", "RAN", "HTTP 500"},
		{"2xx plain text", 200, "not json", "", "RAN", "the response is not a JSON object"},
		{"unreachable", 0, "", closed, "RAN", "connect ECONNREFUSED " + strings.TrimSuffix(strings.TrimPrefix(closed, "http://"), "/hook")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "config")
			var mu sync.Mutex
			var got []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				mu.Lock()
				got = append(got, r.Method+" "+r.Header.Get("Content-Type")+" "+string(b))
				mu.Unlock()
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			url := srv.URL + "/hook"
			if tc.url != "" {
				url = tc.url
			}
			write(t, filepath.Join(dir, ".claude", "settings.json"),
				`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"http","url":"`+url+`"}]}]}}`, 0o644)
			sc := script(t, dir, "s", toolUse("b1", "Bash", `{"command":"echo RAN"}`))
			out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "ht-1",
				"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
			require.Equal(t, 0, code, out)

			recs := readRecs(t, transcriptPath(t, cfg, dir, "ht-1"))
			block, _ := toolResultOf(t, recs, "b1turn-s-a")
			assert.Contains(t, block["content"], tc.want)
			var nonBlocking []map[string]any
			for _, r := range recs {
				if r.Type == "attachment" && r.Attachment["type"] == "hook_non_blocking_error" {
					nonBlocking = append(nonBlocking, r.Attachment)
				}
			}
			if tc.errMsg == "" {
				assert.Empty(t, nonBlocking)
			} else {
				require.Len(t, nonBlocking, 1)
				assert.Equal(t, tc.errMsg, nonBlocking[0]["stderr"])
				assert.Equal(t, "PreToolUse:Bash", nonBlocking[0]["hookName"])
				assert.EqualValues(t, 0, nonBlocking[0]["exitCode"])
			}
			if tc.url == "" {
				mu.Lock()
				defer mu.Unlock()
				require.Len(t, got, 1)
				method, rest, _ := strings.Cut(got[0], " ")
				ctype, body, _ := strings.Cut(rest, " ")
				assert.Equal(t, "POST", method)
				assert.Equal(t, "application/json", ctype)
				var payload map[string]any
				require.NoError(t, json.Unmarshal([]byte(body), &payload))
				assert.Equal(t, "PreToolUse", payload["hook_event_name"])
				assert.Equal(t, "Bash", payload["tool_name"])
			}
		})
	}
}

// TestT017_48_HTTPHookTimeout: an endpoint that answers after the hook's
// timeout is cancelled like a command is: the late answer (a refusal) is
// discarded, the call goes ahead and the transcript records the cancellation.
// sr:proves http-hooks/claude
// sr:proves hook-timeout/claude
func TestT017_48_HTTPHookTimeout(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(5 * time.Second):
		case <-r.Context().Done():
		}
		fmt.Fprint(w, `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"LATE"}}`)
	}))
	defer srv.Close()
	write(t, filepath.Join(dir, ".claude", "settings.json"),
		`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"http","url":"`+srv.URL+`","timeout":1}]}]}}`, 0o644)
	sc := script(t, dir, "s", toolUse("b1", "Bash", `{"command":"echo RAN"}`))
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "ht-2",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "ht-2"))
	block, _ := toolResultOf(t, recs, "b1turn-s-a")
	assert.Equal(t, "RAN", block["content"])
	var cancelled map[string]any
	for _, r := range recs {
		if r.Type == "attachment" && r.Attachment["type"] == "hook_cancelled" {
			cancelled = r.Attachment
		}
	}
	require.NotNil(t, cancelled)
	assert.Equal(t, true, cancelled["timedOut"])
	assert.EqualValues(t, 1000, cancelled["timeoutMs"])
}
