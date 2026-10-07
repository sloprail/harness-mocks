package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skillProject is a project with the skill "greet", whose SKILL.md has front matter and a body, and a
// hook logging every PreToolUse and PostToolUse payload to hooks.log.
func skillProject(t *testing.T) (dir, hookLog string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir()) // the mock reports the project's real path
	require.NoError(t, err)
	hookLog = filepath.Join(dir, "hooks.log")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude", "skills", "greet"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "skills", "greet", "SKILL.md"),
		[]byte("---\nname: greet\ndescription: Say a greeting.\n---\n\nReply with HELLO.\n"), 0o644))
	hook := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\ncat >> "+hookLog+"\necho >> "+hookLog+"\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"),
		[]byte(`{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"`+hook+`"}]}],"PostToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"`+hook+`"}]}]}}`), 0o644))
	return dir, hookLog
}

// launch runs a script that calls Skill once per input, in order, and returns the run's output.
func launch(t *testing.T, dir string, inputs ...string) string {
	t.Helper()
	body := "#!/bin/sh\nF=\"$A10N_MOCK_SESSION_FILE\"\n"
	for i, in := range inputs {
		call := fmt.Sprintf(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"sk%d","name":"Skill","input":%s}]}}`, i, in)
		body += fmt.Sprintf("if ! grep -q '\"id\":\"sk%d\"' \"$F\" 2>/dev/null; then\nprintf '%%s\\n' '%s'\nexit 0\nfi\n", i, call)
	}
	body += `printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'` + "\n"
	script := filepath.Join(dir, ".scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(body), 0o755))
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "skill-1", "--project-dir", dir, "--config-dir", filepath.Join(dir, ".cfg"), "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	return out
}

// TestT020_01_LaunchingASkillPutsItsInstructionsBeforeTheAgent: the call's result says "Launching skill:
// <name>" with a structured result of success and commandName; the skill's body (its front matter left
// out) follows as a synthetic user message headed by the skill's folder, and the hooks see the call and
// that result (recorded: runs/skill-tool).
// sr:proves skill-tool/claude
func TestT020_01_LaunchingASkillPutsItsInstructionsBeforeTheAgent(t *testing.T) {
	dir, hookLog := skillProject(t)
	out := launch(t, dir, `{"skill":"greet"}`)
	assert.Contains(t, out, `"content":"Launching skill: greet"`)
	assert.Contains(t, out, `"tool_use_result":{"commandName":"greet","success":true}`)
	assert.Contains(t, out, `"isSynthetic":true`)
	assert.Contains(t, out, `"text":"Base directory for this skill: `+dir+`/.claude/skills/greet\n\nReply with HELLO.\n"`)
	assert.NotContains(t, out, "description: Say a greeting")
	hooks, err := os.ReadFile(hookLog)
	require.NoError(t, err)
	assert.Contains(t, string(hooks), `"hook_event_name":"PreToolUse","tool_name":"Skill","tool_input":{"skill":"greet"}`)
	assert.Contains(t, string(hooks), `"tool_response":{"commandName":"greet","success":true}`)
}

// TestT020_02_ArgumentsComeAfterTheInstructions: the call's args follow the skill's body as
// "ARGUMENTS: <args>"; a skill launched before is announced as a re-invocation ahead of its
// instructions, which come in full again, with other arguments or the same (recorded: runs/skill-tool).
// sr:proves skill-tool/claude
func TestT020_02_ArgumentsComeAfterTheInstructions(t *testing.T) {
	dir, _ := skillProject(t)
	out := launch(t, dir, `{"skill":"greet"}`, `{"skill":"greet","args":"to the world"}`)
	assert.Equal(t, 1, strings.Count(out, "(Re-invocation of /greet"), "only the second launch is a re-invocation")
	assert.Equal(t, 2, strings.Count(out, "Base directory for this skill"), "each launch brings the instructions in full")
	assert.Contains(t, out, "(Re-invocation of /greet — the skill instructions were previously loaded; the arguments or dynamic output below are new.)")
	assert.Contains(t, out, `Reply with HELLO.\n\n\nARGUMENTS: to the world"`)
}

// TestT020_03_AnUnknownSkillIsAnErrorBeforeAnyHook: a skill that is not there is an error result, "Unknown
// skill: <name>", answered before a hook sees the call (recorded: runs/skill-tool).
// sr:proves skill-tool/claude
func TestT020_03_AnUnknownSkillIsAnErrorBeforeAnyHook(t *testing.T) {
	dir, hookLog := skillProject(t)
	out := launch(t, dir, `{"skill":"no-such-skill"}`)
	assert.Contains(t, out, `"is_error":true`)
	assert.Contains(t, out, `<tool_use_error>Unknown skill: no-such-skill</tool_use_error>`)
	assert.Contains(t, out, `"tool_use_result":"Error: Unknown skill: no-such-skill"`)
	assert.NoFileExists(t, hookLog, "no hook fired")
}

// TestT020_04_TheSameLaunchAgainIsAnnouncedAndGivenInFull: launching a skill a second time with the very
// same input is a re-invocation as well: its notice, and the instructions in full again (recorded:
// runs/skill-tool, steps 3 and 4).
// sr:proves skill-tool/claude
func TestT020_04_TheSameLaunchAgainIsAnnouncedAndGivenInFull(t *testing.T) {
	dir, _ := skillProject(t)
	out := launch(t, dir, `{"skill":"greet"}`, `{"skill":"greet"}`)
	assert.Equal(t, 1, strings.Count(out, "(Re-invocation of /greet"))
	assert.Equal(t, 2, strings.Count(out, "Base directory for this skill"))
}
