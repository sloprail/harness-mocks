package replay

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShellInner(t *testing.T) {
	for line, want := range map[string]string{
		`/bin/zsh -lc "sleep 5 && echo \"hi\""`: `sleep 5 && echo "hi"`,
		`/bin/bash -c 'echo '\''x'\'''`:         `echo 'x'`,
		`/bin/sh -c ls`:                         `ls`,
		`/bin/zsh -c ls -l`:                     `/bin/zsh -c ls -l`, // more than one word after -c: not the invocation of one command
		`echo "a b"`:                            `echo "a b"`,
		`/usr/bin/zsh -c ls`:                    `/usr/bin/zsh -c ls`,
		`ls | wc`:                               `ls | wc`,
	} {
		assert.Equal(t, want, shellInner(line), line)
	}
}

func TestJobProcesses(t *testing.T) {
	ps := `  PID TTY           TIME CMD
  101 ttys001 S 0:00.01 /bin/zsh -c sleep 30
  102 ttys001 S 0:00.00 sleep 30
  103 ttys001 S 0:12.40 /usr/bin/go test ./...
  104 ?? S 0:00.00 sleep abc
`
	assert.Equal(t, "<ps: <job> <SHELL> -c sleep 30 | <job> sleep 30>", jobProcesses(ps))
	assert.Equal(t, "<ps: >", jobProcesses("  1 ttys001 Ss 0:00 ls\n"))
	assert.Equal(t, "not a listing", jobProcesses("not a listing"))
}
