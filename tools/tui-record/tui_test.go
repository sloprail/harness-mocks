package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeTUI is a program that behaves enough like a TUI to be driven: a startup dialog answered by
// one key, a prompt line read and logged as a hook payload, an answer printed, and the same
// again until it is interrupted.
const fakeTUI = `#!/bin/bash
if [ "$1" = --version ]; then echo "fake 1.2.3-abc"; exit 0; fi
[ -z "$FAKE_EXIT" ] || exit 4
printf 'Trust this folder? [y/n] '
read -r -n1 k; echo
[ "$k" = y ] || exit 3
trap 'echo bye; exit 0' INT
echo READY
while read -r line; do
  printf '{"hook_event_name":"beforeSubmitPrompt","prompt":"%s"}\n' "$line" >>"$HOOK_LOG"
  echo "ANSWER-TO $line"
  printf '{"hook_event_name":"stop","loop_count":0}\n' >>"$HOOK_LOG"
done
`

func need(t *testing.T, tool string) {
	t.Helper()
	if _, err := exec.LookPath(tool); err != nil {
		t.Fatalf("%s is required by this test: %v", tool, err)
	}
}

type rig struct {
	dir, bin, log, hooks string
	cfg                  Config
}

func newRig(t *testing.T) *rig {
	t.Helper()
	need(t, "bash")
	dir := t.TempDir()
	r := &rig{dir: dir, bin: filepath.Join(dir, "fake-tui"), log: filepath.Join(dir, "tui.jsonl"), hooks: filepath.Join(dir, "hooks.jsonl")}
	if err := os.WriteFile(r.bin, []byte(fakeTUI), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"home", "repo", "tmp"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	r.cfg = Config{Bin: r.bin, Dir: filepath.Join(dir, "repo"), Home: filepath.Join(dir, "home"), Tmp: filepath.Join(dir, "tmp"), HookLog: r.hooks}
	return r
}

func (r *rig) script(t *testing.T, body string) *Script {
	t.Helper()
	p := filepath.Join(r.dir, "tui.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sc, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return sc
}

const goodScript = `
timeout: 20s
handlers:
  - screen: "Trust this folder"
    send: {keys: [y]}
steps:
  - wait: {screen: "READY"}
  - send: {text: "hello world", keys: [enter]}
  - wait: {hook: stop}
  - send: {text: "second", keys: [enter]}
  - wait: {hook: stop, nth: 2}
  - wait: {screen: "ANSWER-TO second"}
exit:
  - send: {keys: [ctrl-c]}
  - wait: {screen: "bye"}
`

// run plays a script, returning what was logged and the result.
func (r *rig) play(t *testing.T, sc *Script) (string, Result, error) {
	t.Helper()
	var log bytes.Buffer
	r.cfg.Log = &log
	res, err := Run(&r.cfg, sc)
	return log.String(), res, err
}

func TestAScriptedSessionRunsToItsEnd(t *testing.T) {
	r := newRig(t)
	sc := r.script(t, goodScript)
	log, res, err := r.play(t, sc)
	if err != nil {
		t.Fatal(err)
	}
	if res.Killed || res.Exit != 0 {
		t.Fatalf("result %+v", res)
	}
	hooks, _ := os.ReadFile(r.hooks)
	if got := strings.Count(string(hooks), `"stop"`); got != 2 {
		t.Fatalf("hook log:\n%s", hooks)
	}
	if !strings.Contains(log, `"handler":"Trust this folder"`) || !strings.Contains(log, `"nth":2,"step":5,"wait_hook":"stop"`) {
		t.Fatalf("log:\n%s", log)
	}
}

// Two runs of one script leave the same structure: the same steps in the same order and the same
// hook events (the log carries no time or id).
func TestTwoRunsOfOneScriptLogTheSameStructure(t *testing.T) {
	body := goodScript
	var logs, events []string
	for i := 0; i < 2; i++ {
		r := newRig(t)
		log, _, err := r.play(t, r.script(t, body))
		if err != nil {
			t.Fatal(err)
		}
		hooks, _ := os.ReadFile(r.hooks)
		logs, events = append(logs, log), append(events, string(hooks))
	}
	if logs[0] != logs[1] || events[0] != events[1] {
		t.Fatalf("runs differ:\n%s\n--\n%s\n%s\n--\n%s", logs[0], logs[1], events[0], events[1])
	}
}

// A wait that never matches fails at its bound, and says where the screen was.
func TestAWaitThatNeverMatchesFailsAtItsBound(t *testing.T) {
	r := newRig(t)
	sc := r.script(t, "timeout: 20s\nhandlers:\n  - {screen: 'Trust this', send: {keys: [y]}}\nsteps:\n  - wait: {screen: 'NEVER-SHOWN', timeout: 300ms}\n")
	_, res, err := r.play(t, sc)
	if err == nil || !strings.Contains(err.Error(), "NEVER-SHOWN") || !strings.Contains(err.Error(), "READY") {
		t.Fatalf("err = %v", err)
	}
	if !res.Killed {
		t.Fatal("the program was left running")
	}
}

// A hook that is never logged fails at its bound too.
func TestAHookThatIsNeverLoggedFailsAtItsBound(t *testing.T) {
	r := newRig(t)
	sc := r.script(t, "handlers:\n  - {screen: 'Trust this', send: {keys: [y]}}\nsteps:\n  - wait: {hook: stop, timeout: 300ms}\n")
	if _, _, err := r.play(t, sc); err == nil || !strings.Contains(err.Error(), "hook stop") {
		t.Fatalf("err = %v", err)
	}
}

// A program that exits early ends the wait at once, not at its bound.
func TestAProgramThatExitsEndsTheWait(t *testing.T) {
	r := newRig(t)
	r.cfg.Env = []string{"FAKE_EXIT=1"}
	sc := r.script(t, "steps:\n  - wait: {screen: 'NEVER-SHOWN', timeout: 30s}\n")
	if _, _, err := r.play(t, sc); err == nil || !strings.Contains(err.Error(), "exited") {
		t.Fatalf("err = %v", err)
	}
}

func TestOnlyThePinnedBinaryIsRun(t *testing.T) {
	r := newRig(t)
	if err := checkVersion(r.bin, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	if err := checkVersion(r.bin, "1.2.4"); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("a binary of another version: %v", err)
	}
	if err := checkVersion("fake-tui", "1.2.3"); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("a name looked up on PATH: %v", err)
	}
}

func TestOnlyTheLoginIsLaidIntoTheScratchHome(t *testing.T) {
	r := newRig(t)
	src := filepath.Join(r.dir, "auth.json")
	if err := os.WriteFile(src, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := lay(r.cfg.Home, []string{".codex/auth.json=" + src}, false); err != nil {
		t.Fatal(err)
	}
	if err := lay(r.cfg.Home, []string{"Library/Keychains=" + r.dir}, true); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"../escape=" + src, "/abs=" + src, "nosource"} {
		if lay(r.cfg.Home, []string{bad}, true) == nil {
			t.Fatalf("%q was laid", bad)
		}
	}
	env := strings.Join(environment(&r.cfg), "\n")
	if strings.Contains(env, "CLAUDECODE") || !strings.Contains(env, "HOME="+r.cfg.Home) {
		t.Fatalf("environment:\n%s", env)
	}
}

func TestAScriptIsChecked(t *testing.T) {
	r := newRig(t)
	for name, body := range map[string]string{
		"both":        "steps:\n  - wait: {screen: a}\n    send: {keys: [enter]}\n",
		"neither":     "steps:\n  - {}\n",
		"no such key": "steps:\n  - send: {keys: [hyper]}\n",
		"two waits":   "steps:\n  - wait: {screen: a, hook: stop}\n",
		"bad regexp":  "steps:\n  - wait: {screen: '('}\n",
		"unknown":     "stepz: []\n",
	} {
		p := filepath.Join(r.dir, "bad.yaml")
		_ = os.WriteFile(p, []byte(body), 0o644)
		if _, err := Load(p); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// The screen is plain text whatever the program drew with: escapes and a redraw's cursor moves
// are dropped, whitespace collapses, a character cut by a chunk is whole, and a query is answered.
func TestScreenKeepsOnlyTheText(t *testing.T) {
	var answers []string
	s := NewScreen(func(a string) { answers = append(answers, a) })
	s.Write([]byte("\x1b[2K\x1b[1A\x1b[1;32mHello\x1b[0m,\r\n   wor"))
	s.Write([]byte("ld \x1b]0;title\x07\xe2"))
	s.Write([]byte("\x86\x92 ok\x1b[6n\x1b[c"))
	got, _ := s.Since(0)
	if got != "Hello, world → ok" {
		t.Fatalf("screen %q", got)
	}
	if strings.Join(answers, "|") != "\x1b[1;1R|\x1b[?62;c" {
		t.Fatalf("answers %q", answers)
	}
}
