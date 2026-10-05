package toolexec

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// mcpClient is a started server and the requests made of it.
type mcpClient struct {
	cmd *exec.Cmd
	in  *json.Encoder
	out *bufio.Scanner
	id  int
}

func startMCP(ctx context.Context, dir, server string) (*mcpClient, error) {
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
	cmd := exec.CommandContext(ctx, s.Command, s.Args...)
	cmd.Dir = dir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cursor-mock: MCP server %q: %w", server, err)
	}
	c := &mcpClient{cmd: cmd, in: json.NewEncoder(stdin), out: bufio.NewScanner(stdout)}
	c.out.Buffer(make([]byte, 1<<20), 1<<20)
	if _, err := c.call("initialize", map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "cursor-mock", "version": "1"}}); err != nil {
		c.close()
		return nil, err
	}
	_ = c.in.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return c, nil
}

func (c *mcpClient) close() { _ = c.cmd.Process.Kill(); _ = c.cmd.Wait() }

// call makes one request and returns its result.
func (c *mcpClient) call(method string, params any) (json.RawMessage, error) {
	c.id++
	if err := c.in.Encode(map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for c.out.Scan() {
		var r struct {
			ID     *int            `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(c.out.Bytes(), &r) != nil || r.ID == nil || *r.ID != c.id {
			continue
		}
		if r.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, r.Error.Message)
		}
		return r.Result, nil
	}
	return nil, fmt.Errorf("cursor-mock: the MCP server ended before answering %s", method)
}
