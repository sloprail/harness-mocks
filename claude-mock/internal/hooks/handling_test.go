package hooks

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func handler(cmd string) HandlerSpec { return HandlerSpec{Type: "command", Command: cmd} }

func entriesWith(matcher string) *Settings {
	return &Settings{Hooks: map[EventName][]HookEntry{
		EventPreToolUse: {{Matcher: matcher, Hooks: []HandlerSpec{handler("H")}}},
	}}
}

// A matcher of plain names is exact (a list with | or ,, spaces allowed); any
// other is a regular expression searched in the tool name, unanchored; an
// empty or "*" matcher takes everything (docs, Matcher patterns).
// sr:proves hook-matcher-filter/claude
func TestEntriesFor_MatcherForms(t *testing.T) {
	for _, tc := range []struct {
		matcher, tool string
		want          bool
	}{
		{"", "Bash", true}, {"*", "Bash", true}, {"Bash", "Bash", true}, {"Bash", "BashX", false}, {"bash", "Bash", false},
		{"Edit|Write", "Write", true}, {"Edit, Write", "Edit", true}, {"Edit|Write", "NotebookEdit", false},
		{"Edit.*", "NotebookEdit", true},  // a regexp is unanchored: it also matches NotebookEdit
		{"^Edit$", "NotebookEdit", false}, // and anchors when asked to
		{"^Notebook", "NotebookEdit", true},
		{"mcp__memory", "mcp__memory__read", false}, // plain characters only: an exact name, so no tool
		{"mcp__memory__.*", "mcp__memory__read", true},
		{"mcp__brave-search__.*", "mcp__brave-search__web", true},
		{"code-reviewer", "code-reviewer", true}, {"Bash(git *)", "Bash", false},
	} {
		got := len(entriesWith(tc.matcher).EntriesFor(EventPreToolUse, tc.tool)) == 1
		assert.Equal(t, tc.want, got, "matcher %q on tool %q", tc.matcher, tc.tool)
	}
	// an event with no subject takes every hook whatever its matcher
	s := &Settings{Hooks: map[EventName][]HookEntry{EventStop: {{Matcher: "NoSuchThing", Hooks: []HandlerSpec{handler("H")}}}}}
	assert.Len(t, s.EntriesFor(EventStop, matcherSubject(Input{HookEventName: EventStop})), 1)
}

// Each event filters on its own field (docs, Matcher patterns).
// sr:proves hook-matcher-filter/claude
func TestMatcherSubject(t *testing.T) {
	for _, tc := range []struct {
		in   Input
		want string
	}{
		{Input{HookEventName: EventPreToolUse, ToolName: "Bash"}, "Bash"},
		{Input{HookEventName: EventPostToolUse, ToolName: "Read"}, "Read"},
		{Input{HookEventName: EventPostToolUseFailure, ToolName: "Edit"}, "Edit"},
		{Input{HookEventName: EventSessionStart, Source: "resume"}, "resume"},
		{Input{HookEventName: EventSessionEnd, Reason: "other"}, "other"},
		{Input{HookEventName: EventSubagentStart, AgentType: "Explore"}, "Explore"},
		{Input{HookEventName: EventSubagentStop, AgentType: "Plan"}, "Plan"},
		{Input{HookEventName: EventPreCompact, Trigger: "manual"}, "manual"},
		{Input{HookEventName: EventPostCompact, Trigger: "auto"}, "auto"},
		{Input{HookEventName: EventStopFailure, Error: "rate_limit"}, "rate_limit"},
		{Input{HookEventName: EventUserPromptSubmit, Prompt: "p"}, ""},
		{Input{HookEventName: EventStop}, ""},
	} {
		assert.Equal(t, tc.want, matcherSubject(tc.in), "%s", tc.in.HookEventName)
	}
}

// StopFailure filters on its error type, with the narrower exact-match set: a
// hyphen, space or comma makes the matcher a regular expression, not a list.
// sr:proves hook-matcher-filter/claude
func TestEntriesFor_StopFailureMatcher(t *testing.T) {
	for _, tc := range []struct {
		matcher, errType string
		want             bool
	}{
		{"", "rate_limit", true}, {"*", "server_error", true},
		{"rate_limit", "rate_limit", true}, {"rate_limit", "server_error", false},
		{"rate_limit|server_error", "server_error", true}, {"rate_limit|server_error", "billing_error", false},
		{"rate", "rate_limit", false},                      // plain characters: an exact name
		{"rate.*", "rate_limit", true},                     // a regexp searches the error type
		{"rate_limit,server_error", "server_error", false}, // a comma makes it a regexp here, not a list
	} {
		s := &Settings{Hooks: map[EventName][]HookEntry{EventStopFailure: {{Matcher: tc.matcher, Hooks: []HandlerSpec{handler("H")}}}}}
		got := len(s.EntriesFor(EventStopFailure, matcherSubject(Input{HookEventName: EventStopFailure, Error: tc.errType}))) == 1
		assert.Equal(t, tc.want, got, "matcher %q on error %q", tc.matcher, tc.errType)
	}
}

// A hook with no timeout of its own may run 600 seconds, 30 before a prompt, and
// a session-end hook shares a 1.5-second budget; its own timeout wins, and a
// session-end hook's is capped at 60 seconds (docs, Common fields).
// sr:proves hook-timeout/claude
func TestTimeouts(t *testing.T) {
	assert.Equal(t, 600*time.Second, defaultTimeout(EventPreToolUse))
	assert.Equal(t, 600*time.Second, defaultTimeout(EventStop))
	assert.Equal(t, 30*time.Second, defaultTimeout(EventUserPromptSubmit))
	assert.Equal(t, 1500*time.Millisecond, defaultTimeout(EventSessionEnd))
	assert.Equal(t, 5*time.Second, commandTimeout(HandlerSpec{Timeout: 5}, EventPreToolUse))
	assert.Equal(t, 300*time.Second, commandTimeout(HandlerSpec{Timeout: 300}, EventPreToolUse))
	assert.Equal(t, 60*time.Second, commandTimeout(HandlerSpec{Timeout: 300}, EventSessionEnd))
	assert.Equal(t, time.Duration(0), commandTimeout(HandlerSpec{}, EventSessionEnd))
}

// The same handler under the same matcher in several settings files runs once;
// a different matcher or handler is kept (docs, Hook handler fields).
// sr:proves hooks-all-matching-run/claude
func TestMergeEntriesRunsTheSameHandlerOnce(t *testing.T) {
	have := []HookEntry{{Matcher: "Bash", Hooks: []HandlerSpec{handler("A")}}}
	got := mergeEntries(have, []HookEntry{
		{Matcher: "Bash", Hooks: []HandlerSpec{handler("A"), handler("B")}}, // A repeats, B is new
		{Matcher: "Read", Hooks: []HandlerSpec{handler("A")}},               // same command, another matcher
		{Matcher: "Bash", Hooks: []HandlerSpec{handler("A")}},               // all repeats: no entry at all
	})
	var flat []string
	for _, e := range got {
		for _, h := range e.Hooks {
			flat = append(flat, e.Matcher+":"+h.Command)
		}
	}
	assert.Equal(t, []string{"Bash:A", "Bash:B", "Read:A"}, flat)
	// a plugin's copy of a handler stays separate: it names where it came from
	withPlugin := handler("A")
	withPlugin.PluginRoot = "/p"
	assert.Len(t, mergeEntries(have, []HookEntry{{Matcher: "Bash", Hooks: []HandlerSpec{withPlugin}}}), 2)
}

// A plugin's hook is told where its plugin is installed and where its data
// lives; a project hook is told neither (docs, Reference scripts by path).
// sr:proves plugin-hooks/claude
func TestPluginHookEnvironment(t *testing.T) {
	assert.Nil(t, handler("A").env(""))
	h := handler("A")
	h.PluginRoot, h.PluginData = "/p/root", "/p/data"
	assert.Equal(t, []string{"CLAUDE_PLUGIN_ROOT=/p/root", "CLAUDE_PLUGIN_DATA=/p/data"}, h.env(""))
}

// A command hook runs in the event's working directory; when that is gone it
// falls back to the session's start directory, the project root, the home
// directory, the temp directory (docs, Hook handler fields).
// sr:proves hook-command-handler/claude
func TestHookDirFallsBack(t *testing.T) {
	live, project := t.TempDir(), t.TempDir()
	gone := live + "/deleted"
	assert.Equal(t, live, hookDir(live, project))
	assert.Equal(t, project, hookDir(gone, "", project), "the first fallback that exists")
	assert.Equal(t, live, hookDir(gone, live, project), "the session's start directory comes first")
	home, _ := os.UserHomeDir()
	assert.Equal(t, home, hookDir(gone, gone+"2", gone+"3"), "then the home directory")
	t.Setenv("HOME", gone+"4") // no home either: the system temp directory is the last resort
	assert.Equal(t, os.TempDir(), hookDir(gone, gone+"2", gone+"3"), "and last the temp directory")
}

// A handler repeated across the settings files the loader reads runs once.
// sr:proves hooks-all-matching-run/claude
func TestLoadSettingsDedupesTheSameHandlerAcrossFiles(t *testing.T) {
	repo := t.TempDir()
	writeClaudeSettings(t, repo, "settings.json",
		`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"SAME"},{"type":"http","url":"http://x"}]}]}}`)
	writeClaudeSettings(t, repo, "settings.local.json",
		`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"SAME"},{"type":"http","url":"http://x"},{"type":"command","command":"OTHER"}]}]}}`)
	got, err := LoadSettings(repo, t.TempDir())
	assert.NoError(t, err)
	var cmds []string
	for _, h := range got.EntriesFor(EventPreToolUse, "Bash") {
		cmds = append(cmds, h.Type+":"+h.Command+h.URL)
	}
	assert.Equal(t, []string{"command:SAME", "http:http://x", "command:OTHER"}, cmds)
}

// A SessionStart hook is told CLAUDE_ENV_FILE, under the config dir's session-env folder of the
// session, one file per hook; no other event's hook is.
// (recorded: runs/subprocess-session-env)
func TestSessionStartHookIsToldItsEnvFile(t *testing.T) {
	inv := &Invoker{sessionID: "s1", configDir: "/cfg"}
	assert.Equal(t, "/cfg/session-env/s1/sessionstart-hook-1.sh", inv.envFile(EventSessionStart, 1))
	assert.Empty(t, inv.envFile(EventPreToolUse, 0))
	assert.Equal(t, []string{"CLAUDE_ENV_FILE=/f"}, handler("A").env("/f"))
}
