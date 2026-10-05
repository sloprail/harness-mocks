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
	h := toolHost{state: &state{cfg: Config{Cwd: t.TempDir()}}}
	assert.Equal(t, "", h.unimplemented(call(`{"command":"x","shell":"zsh","login":true,"max_output_tokens":9}`)))
	assert.Equal(t, "", h.unimplemented(call(`{"command":"x","workdir":"`+h.cfg.Cwd+`"}`)))
	assert.NotEqual(t, "", h.unimplemented(call(`{"command":"x","workdir":"/"}`)))
}
