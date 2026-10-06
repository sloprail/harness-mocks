package toolexec

import (
	"context"
	"sync"
)

type agentKey struct{}

// WithAgent says which agent the tool calls made under ctx are of: each agent has its own context, so
// what one has read or written is not in another's.
func WithAgent(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, agentKey{}, agentID)
}

// known is, per agent and file, whether the agent's context holds the file as it now is: it wrote it
// (Write) or read all of it (Read without an offset or limit), and has not read only part of it since.
var known = struct {
	sync.Mutex
	files map[string]bool
}{files: map[string]bool{}}

func fileKey(ctx context.Context, sessionID, path string) string {
	agent, _ := ctx.Value(agentKey{}).(string)
	return sessionID + "\x00" + agent + "\x00" + path
}

func setKnown(ctx context.Context, sessionID, path string, v bool) {
	known.Lock()
	known.files[fileKey(ctx, sessionID, path)] = v
	known.Unlock()
}

func isKnown(ctx context.Context, sessionID, path string) bool {
	known.Lock()
	defer known.Unlock()
	return known.files[fileKey(ctx, sessionID, path)]
}
