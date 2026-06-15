package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

const defaultHookTimeout = 60 * time.Second

// Invoker fires hook handlers for a given event and collects their output.
type Invoker struct {
	settings *Settings
	cwd      string
}

// NewInvoker creates an Invoker backed by the given settings.
func NewInvoker(settings *Settings, cwd string) *Invoker {
	return &Invoker{settings: settings, cwd: cwd}
}

// Fire invokes all handlers configured for the event and matcher, then returns
// the merged Output. Command hooks that exit 2 are treated as blocking errors
// and returned via the error return. Other non-zero exits are non-blocking
// (logged and ignored).
func (inv *Invoker) Fire(ctx context.Context, input Input) (Output, error) {
	handlers := inv.settings.EntriesFor(input.HookEventName, input.ToolName)
	if len(handlers) == 0 {
		return Output{}, nil
	}

	payload, err := json.Marshal(input)
	if err != nil {
		return Output{}, fmt.Errorf("hooks: marshal input: %w", err)
	}

	var merged Output
	for _, h := range handlers {
		out, blockErr := inv.invoke(ctx, h, payload)
		if blockErr != nil {
			return merged, blockErr
		}
		mergeOutput(&merged, out)
	}
	return merged, nil
}

func (inv *Invoker) invoke(ctx context.Context, h HandlerSpec, payload []byte) (Output, error) {
	timeout := defaultHookTimeout
	if h.Timeout > 0 {
		timeout = time.Duration(h.Timeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch h.Type {
	case "command":
		return inv.invokeCommand(ctx, h, payload)
	case "http":
		return inv.invokeHTTP(ctx, h, payload)
	default:
		slog.Debug("hooks: unsupported handler type", "type", h.Type)
		return Output{}, nil
	}
}

func (inv *Invoker) invokeCommand(ctx context.Context, h HandlerSpec, payload []byte) (Output, error) {
	parts := strings.Fields(h.Command)
	if len(parts) == 0 {
		return Output{}, nil
	}
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...) //nolint:gosec
	cmd.Dir = inv.cwd
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	if exitCode == 2 {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = "hook blocked the action"
		}
		return Output{}, fmt.Errorf("hooks: command blocked: %s", msg)
	}
	if runErr != nil {
		slog.Debug("hooks: command non-blocking error", "cmd", h.Command, "err", runErr, "stderr", stderr.String())
		return Output{}, nil
	}

	var out Output
	if stdout.Len() > 0 {
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			slog.Debug("hooks: command output not valid JSON", "cmd", h.Command, "err", err)
		}
	}
	return out, nil
}

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

func mergeOutput(dst *Output, src Output) {
	if src.Continue != nil {
		dst.Continue = src.Continue
	}
	if src.StopReason != "" {
		dst.StopReason = src.StopReason
	}
	if src.SystemMessage != "" {
		dst.SystemMessage = src.SystemMessage
	}
	if src.Decision != "" {
		dst.Decision = src.Decision
	}
	if src.Reason != "" {
		dst.Reason = src.Reason
	}
	if src.HookSpecificOutput != nil {
		dst.HookSpecificOutput = src.HookSpecificOutput
	}
}
