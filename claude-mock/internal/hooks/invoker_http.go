package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

func (inv *Invoker) invokeHTTP(ctx context.Context, h HandlerSpec, payload []byte) (Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(payload))
	if err != nil {
		return Output{}, nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Debug("hooks: http error (non-blocking)", "url", h.URL, "err", err)
		return Output{}, nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Debug("hooks: http non-2xx (non-blocking)", "url", h.URL, "status", resp.StatusCode)
		return Output{}, nil
	}

	var out Output
	if len(body) > 0 {
		if err := json.Unmarshal(body, &out); err != nil {
			slog.Debug("hooks: http response not valid JSON", "url", h.URL, "err", err)
		}
	}
	return out, nil
}
