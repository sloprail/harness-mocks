package hooks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"syscall"
	"time"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// httpRun runs a hook that is an HTTP endpoint: the event's payload is POSTed
// to it as JSON. Whether it succeeded, decided or failed is core's
// (corehooks.HTTPResult); this reads the response the way Claude Code does.
func (inv *Invoker) httpRun(ctx context.Context, h HandlerSpec, ev EventName, payload []byte) HandlerRun {
	timeout := corehooks.DefaultTimeout(time.Duration(h.Timeout)*time.Second, defaultTimeout(ev))
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	run := HandlerRun{Command: h.URL}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(payload))
	if err != nil {
		run.HTTPError = err.Error()
		return run
	}
	req.Header.Set("Content-Type", "application/json")
	status, body := 0, ""
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		status, body = resp.StatusCode, string(b)
	}
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
	// sr:provides http-hooks/claude
	switch corehooks.HTTPResult(err == nil, timedOut, status, body) {
	case corehooks.HTTPCancelled:
		run.TimedOut, run.TimeoutMs = true, timeout.Milliseconds()
	case corehooks.HTTPDecided:
		if msg := decodeOutput([]byte(body), &run.Output); msg != "" {
			run.HTTPError = msg
		} else {
			run.JSONParsed = true
		}
	case corehooks.HTTPNonBlockingError:
		run.HTTPError = httpFailure(err, status, req.URL.Host)
	}
	return run
}

// httpFailure says why an HTTP hook was a non-blocking error: a refused
// connection in Claude Code's wording (recorded: snapshots/runs/http-hook),
// else the transport's own error or the status.
func httpFailure(err error, status int, host string) string {
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connect ECONNREFUSED " + host
	case err != nil:
		return err.Error()
	case status < 200 || status > 299:
		return fmt.Sprintf("HTTP %d", status)
	}
	return "the response is not a JSON object"
}
