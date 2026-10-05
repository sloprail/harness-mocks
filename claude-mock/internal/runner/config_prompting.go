package runner

import "github.com/sloprail/harness-mocks/claude-mock/internal/hooks"

// Prompting is what hook payloads tell of the prompt a session is on.
type Prompting struct {
	// PermissionMode is the permission_mode the hooks about a turn are told.
	// sr:docs https://code.claude.com/docs/en/hooks#common-input-fields
	PermissionMode string
	// Turn is the prompt the session is on: the root run makes it, every
	// sub-agent run inside shares it.
	Turn *hooks.Turn
}
