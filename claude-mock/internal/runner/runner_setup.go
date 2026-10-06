package runner

import (
	"errors"
	"fmt"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

// loadRunSettings reads the project's settings. A field the mock does not implement refuses the run
// (adr/fail-fast-unimplemented); a file that cannot be read is warned of, and the run goes on with no hooks.
func loadRunSettings(cfg Config) (*hooks.Settings, error) {
	settings, err := hooks.LoadSettings(cfg.ProjectDir, cfg.PluginCacheDir)
	var unimplemented *hooks.UnimplementedError
	if errors.As(err, &unimplemented) {
		return nil, err
	}
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "claude-mock: warn: loading settings: %v\n", err)
		settings = &hooks.Settings{Hooks: make(map[hooks.EventName][]hooks.HookEntry)}
	}
	return settings, nil
}

// refuseUnrecordedHook refuses --include-hook-events when the run reaches a firing of a hook whose
// frames are not recorded and one is configured (adr/fail-fast-unimplemented). What is recorded
// (runs/include-hook-events, include-hook-events-more): UserPromptSubmit, PreToolUse, PostToolUse,
// PostToolUseFailure, Stop, a foreground sub-agent's SubagentStart and SubagentStop, and SessionStart;
// SessionEnd leaves no frame. A hook configured for an event that does not fire is no matter.
func refuseUnrecordedHook(cfg Config, inv *hooks.Invoker, events ...hooks.EventName) error {
	if !cfg.HookEvents {
		return nil
	}
	for _, e := range events {
		if inv.Configured(e) {
			return &hooks.UnimplementedError{What: "--include-hook-events with a " + string(e) + " hook that fires (its frames are not recorded)"}
		}
	}
	return nil
}

// refuseUnrecordedControl is refuseUnrecordedHook for the control records that fire hooks.
func refuseUnrecordedControl(cfg Config, inv *hooks.Invoker, recType string) error {
	switch recType {
	case "worktree_create":
		return refuseUnrecordedHook(cfg, inv, hooks.EventWorktreeCreate)
	case "worktree_remove":
		return refuseUnrecordedHook(cfg, inv, hooks.EventWorktreeRemove)
	}
	return nil
}

// newInvoker is the hook invoker of a run, with what its payloads carry, and the prompt state it
// shares with the run (the root run makes it, each sub-agent run inside shares it).
func newInvoker(cfg Config, settings *hooks.Settings, tr *transcript) (*hooks.Invoker, *hooks.Turn) {
	inv := hooks.NewInvoker(settings, cfg.Cwd, cfg.SessionID)
	cfg.configureInvoker(inv)
	turn := cfg.Turn
	if turn != nil {
		inv.SetTurn(turn)
	} else {
		turn = inv.Turn()
	}
	inv.SetTranscriptPath(tr.reported)
	inv.SetProjectDir(projectDirOf(cfg))
	inv.SetRecorder(tr.recordHookRuns)
	if cfg.AgentID != "" {
		inv.SetAgent(cfg.AgentID, cfg.AgentType)
	}
	return inv, turn
}
