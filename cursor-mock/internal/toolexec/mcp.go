package toolexec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/internal/procexec"
)

// An MCP tool is called by the name a scenario script gives it, mcp__<server>__<tool>
// (the Claude Code form); the server is one of the project's .cursor/mcp.json,
// a program speaking MCP over its stdio, which the mock starts for the call.
// Recorded (runs/hook-matchers-mcp): the call is preceded by a getMcpToolsToolCall
// (the agent reading the tool's description and schema), and its hooks name the
// tool MCP:<tool>.

// mcpName splits an MCP tool's script name into its server and tool.
func mcpName(name string) (server, tool string, ok bool) {
	rest, ok := strings.CutPrefix(name, "mcp__")
	if !ok {
		return "", "", false
	}
	server, tool, ok = strings.Cut(rest, "__")
	return server, tool, ok && server != "" && tool != ""
}

type mcpServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// mcpRequest is one request of an exchange with a server.
type mcpRequest struct {
	method string
	params any
}

// mcpExchange starts the project's server, sends it the handshake and the
// requests, closes its input, and returns the result of each request in order.
// A stdio server ends when its input does, so one run of it answers them all.
func mcpExchange(ctx context.Context, dir, server string, env []string, reqs ...mcpRequest) ([]json.RawMessage, error) {
	b, err := os.ReadFile(filepath.Join(dir, ".cursor", "mcp.json"))
	if err != nil {
		return nil, fmt.Errorf("cursor-mock: no MCP servers are configured (.cursor/mcp.json): %w", err)
	}
	var cfg struct {
		Servers map[string]mcpServer `json:"mcpServers"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("cursor-mock: .cursor/mcp.json: %w", err)
	}
	s, ok := cfg.Servers[server]
	if !ok {
		return nil, fmt.Errorf("cursor-mock: no MCP server %q is configured", server)
	}
	var in bytes.Buffer
	enc := json.NewEncoder(&in)
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "cursor-mock", "version": "1"}}})
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	for i, r := range reqs {
		_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": i + 1, "method": r.method, "params": r.params})
	}
	res, err := procexec.Run(ctx, procexec.Spec{Argv: append([]string{s.Command}, s.Args...), Dir: dir, Stdin: in.Bytes(), Env: env, Timeout: 20 * time.Second})
	if err != nil || !res.Started {
		return nil, fmt.Errorf("cursor-mock: MCP server %q could not be run: %v", server, err)
	}
	answers := map[int]json.RawMessage{}
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		var r struct {
			ID     *int            `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(line), &r) != nil || r.ID == nil {
			continue
		}
		if r.Error != nil {
			return nil, fmt.Errorf("MCP server %q: %s", server, r.Error.Message)
		}
		answers[*r.ID] = r.Result
	}
	out := make([]json.RawMessage, len(reqs))
	for i, r := range reqs {
		a, ok := answers[i+1]
		if !ok {
			return nil, fmt.Errorf("cursor-mock: the MCP server %q did not answer %s", server, r.method)
		}
		out[i] = a
	}
	return out, nil
}
