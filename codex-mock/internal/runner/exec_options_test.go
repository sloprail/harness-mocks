package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sloprail/harness-mocks/internal/toolcall"
)

func call(in string) toolcall.Call { return toolcall.Call{Input: []byte(in)} }

func TestTooLongIsTheOutputPastMaxOutputTokens(t *testing.T) {
	assert.False(t, tooLong(call(`{"command":"x"}`), string(make([]byte, 10000))))
	assert.False(t, tooLong(call(`{"command":"x","max_output_tokens":100}`), string(make([]byte, 400))))
	assert.True(t, tooLong(call(`{"command":"x","max_output_tokens":100}`), string(make([]byte, 401))))
}

func TestATtyEndsLinesWithCRLF(t *testing.T) {
	assert.Equal(t, "a\r\nb\r\n", ttyOutput(call(`{"command":"x","tty":true}`), "a\nb\n"))
	assert.Equal(t, "a\r\n", ttyOutput(call(`{"command":"x","tty":true}`), "a\r\n"))
	assert.Equal(t, "a\n", ttyOutput(call(`{"command":"x"}`), "a\n"))
}

func TestUnimplementedNamesAnotherDirectory(t *testing.T) {
	old := zsh
	defer func() { zsh = old }()
	zsh = func() string { return "/usr/bin/zsh" } // the test is about what is refused, not about this machine having zsh
	h := toolHost{state: &state{cfg: Config{Cwd: t.TempDir()}}}
	assert.Equal(t, "", h.unimplemented(call(`{"command":"x","shell":"zsh","login":true,"max_output_tokens":9}`)))
	assert.Equal(t, "", h.unimplemented(call(`{"command":"x","workdir":"`+h.cfg.Cwd+`"}`)))
	assert.NotEqual(t, "", h.unimplemented(call(`{"command":"x","workdir":"/"}`)))
	assert.Contains(t, h.unimplemented(call(`{"command":"x","shell":"fish"}`)), "shell fish")
}

func TestACommandRunsByTheShellItNames(t *testing.T) {
	old := zsh
	defer func() { zsh = old }()
	zsh = func() string { return "/usr/bin/zsh" }
	assert.Equal(t, []string{"/bin/sh", "-c", "x"}, shellArgv(call(`{"command":"x"}`), "x"))
	assert.Equal(t, []string{"/usr/bin/zsh", "-c", "x"}, shellArgv(call(`{"command":"x","shell":"zsh","login":false}`), "x"))
	assert.Equal(t, []string{"/usr/bin/zsh", "-lc", "x"}, shellArgv(call(`{"command":"x","shell":"zsh","login":true}`), "x"))
}

// A named shell that is not installed is refused, never replaced by /bin/sh; so is a login with no shell.
func TestAMissingShellIsRefusedNotReplaced(t *testing.T) {
	old := zsh
	defer func() { zsh = old }()
	zsh = func() string { return "" }
	h := toolHost{state: &state{cfg: Config{Cwd: t.TempDir()}}}
	assert.Contains(t, h.unimplemented(call(`{"command":"x","shell":"zsh"}`)), "zsh is not installed")
	assert.Equal(t, "", h.unimplemented(call(`{"command":"x"}`)))
	assert.Equal(t, "login without a shell", h.unimplemented(call(`{"command":"x","login":true}`)))
	assert.Equal(t, "", h.unimplemented(call(`{"command":"x","login":false}`)), "recorded: runs/exec-login-false")
}
