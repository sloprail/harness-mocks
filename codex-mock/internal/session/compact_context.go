package session

import "time"

// afterCompaction writes what Codex writes after the compacted record: the turn's context again, and
// the thread's settings as applied.
func (s *File) afterCompaction(turn Turn) {
	profile, policy := sandboxRecords(turn.Cwd, turn.Sandbox)
	collab := map[string]any{"mode": "default", "settings": map[string]any{"model": turn.Model, "reasoning_effort": nil, "developer_instructions": nil}}
	zone, _ := time.Now().Zone()
	tc := map[string]any{"turn_id": turn.TurnID, "root_turn_id": turn.TurnID, "disabled_plugin_ids": []any{}, "cwd": turn.Cwd,
		"workspace_roots": []string{turn.Cwd}, "current_date": time.Now().Format("2006-01-02"), "timezone": zone, "approval_policy": "never",
		"approvals_reviewer": "user", "sandbox_policy": policy, "permission_profile": profile, "model": turn.Model, "collaboration_mode": collab,
		"multi_agent_version": "v1", "realtime_active": false, "effort": nil, "summary": "none"}
	// recorded: a workspace-write turn names its file system policy; a run that asked for no sandbox names its profile
	switch turn.Sandbox {
	case "workspace-write":
		tc["file_system_sandbox_policy"] = profile["file_system"]
	case "":
		tc["active_permission_profile"] = map[string]any{"id": ":read-only"}
	}
	s.append("turn_context", tc)
	s.append("event_msg", map[string]any{"type": "thread_settings_applied", "thread_id": turn.SessionID, "thread_settings": map[string]any{
		"model": turn.Model, "model_provider_id": "openai", "approval_policy": "never", "approvals_reviewer": "user", "permission_profile": profile,
		"cwd": turn.Cwd, "runtime_workspace_roots": []string{turn.Cwd}, "reasoning_effort": nil, "collaboration_mode": collab, "disabled_plugin_ids": []any{}}})
}

// sandboxRecords are the permission profile and the sandbox policy Codex records for a sandbox mode
// (recorded: the turn_context of runs with each mode; none asked for is read-only).
func sandboxRecords(cwd, mode string) (profile, policy map[string]any) {
	switch mode {
	case "danger-full-access":
		return map[string]any{"type": "disabled"}, map[string]any{"type": "danger-full-access"}
	case "workspace-write":
		path := func(p, access string) map[string]any {
			return map[string]any{"path": map[string]any{"type": "path", "path": p}, "access": access, "missing_path_behavior": "skip"}
		}
		special := func(kind, access string) map[string]any {
			return map[string]any{"path": map[string]any{"type": "special", "value": map[string]any{"kind": kind}}, "access": access}
		}
		entries := []any{special("root", "read"), map[string]any{"path": map[string]any{"type": "path", "path": cwd}, "access": "write"},
			special("slash_tmp", "write"), special("tmpdir", "write"),
			path(cwd+"/.git", "read"), path(cwd+"/.agents", "read"), path(cwd+"/.codex", "read"), path(cwd+"/.aws", "read")}
		return map[string]any{"type": "managed", "file_system": map[string]any{"type": "restricted", "entries": entries}, "network": "restricted"},
			map[string]any{"type": "workspace-write", "network_access": false, "exclude_tmpdir_env_var": false, "exclude_slash_tmp": false}
	}
	return map[string]any{"type": "managed", "file_system": map[string]any{"type": "restricted", "entries": []any{
			map[string]any{"path": map[string]any{"type": "special", "value": map[string]any{"kind": "root"}}, "access": "read"}}}, "network": "restricted"},
		map[string]any{"type": "read-only"}
}
