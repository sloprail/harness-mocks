package runner

import (
	"regexp"
	"strings"
)

// A compaction streams frames a `claude -p --output-format stream-json` run shows
// around it (recorded: snapshots/runs/compact): system/status "compacting" when
// it starts, system/status with compact_result "success" when the summary is
// done, a system/init frame, and the system/compact_boundary frame carrying the
// boundary's metadata in snake_case. The summary and, for a manual compaction,
// the /compact command's output follow as user frames.

// writeCompactingStatus announces that a compaction has begun.
func writeCompactingStatus(cfg Config) {
	writeFrame(cfg, map[string]any{"type": "system", "subtype": "status", "status": "compacting"})
}

// writeCompactedFrames streams what a finished compaction reports before its
// summary: the end status, a session init and the boundary, whose metadata is the
// transcript boundary's (meta) with its keys in snake_case.
func writeCompactedFrames(cfg Config, meta map[string]any, logicalParent string) {
	writeFrame(cfg, map[string]any{"type": "system", "subtype": "status", "status": nil, "compact_result": "success"})
	writeInitFrame(cfg)
	writeFrame(cfg, map[string]any{
		"type": "system", "subtype": "compact_boundary",
		"compact_metadata": snakeKeys(meta), "logical_parent_uuid": logicalParent,
	})
}

// writeInitFrame streams a system/init frame, which opens a turn that starts from idle: after a
// compaction, and for the turn a background task's notification starts (recorded: runs/bgagent).
func writeInitFrame(cfg Config) {
	init := map[string]any{"type": "system", "subtype": "init", "cwd": cfg.Cwd, "claude_code_version": stampVersion}
	if cfg.Model != "" {
		init["model"] = cfg.Model
	}
	writeFrame(cfg, init)
}

var camelHump = regexp.MustCompile(`([a-z0-9])([A-Z])`)

// snakeKeys is v with every map key camelCase turned to snake_case.
func snakeKeys(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[strings.ToLower(camelHump.ReplaceAllString(k, "${1}_${2}"))] = snakeKeys(val)
		}
		return out
	case []string:
		return x
	}
	return v
}
