package replay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sloprail/harness-mocks/internal/procexec"
	core "github.com/sloprail/harness-mocks/internal/replay"
)

// refusedFlags are the flags of `codex exec` that the mock takes only to refuse (codex-mock/unimplemented.go).
var refusedFlags = map[string]bool{"--enable": true, "--disable": true, "--output-last-message": true, "-o": true,
	"--output-schema": true, "--thread-source": true, "--sandbox": true, "-s": true, "--profile": true, "-p": true,
	"--color": true, "--ignore-user-config": true, "--ignore-rules": true, "--strict-config": true, "--approve-for-me": true}

// refusedFlagIn is the first flag of a recorded run's args that the mock refuses, and the args as words.
func refusedFlagIn(args string) (flag string, words []string) {
	words = strings.Fields(args)
	for _, w := range words {
		if refusedFlags[w] {
			return w, words
		}
	}
	return "", words
}

// checkRefusal replays a recording made with a flag the mock refuses as a check that the mock refuses it
// (adr/refused-flag-replay): the mock is run with the recorded command line and args, and must exit non-zero
// with a refusal naming the flag, never having run the script. The run then counts as replayed: nothing differs.
func checkRefusal(mock string, rec core.Recording, environ []string) error {
	flag, words := rec.Setup["refused-flag"], strings.Fields(rec.Setup["refused-args"])
	root, err := os.MkdirTemp("", "codex-refusal-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	repo, home := filepath.Join(root, "repo"), filepath.Join(root, "home", ".codex")
	for _, d := range []string{repo, home} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	ran := filepath.Join(root, "script-ran")
	script := filepath.Join(root, "scenario.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+ran+"\n"), 0o755); err != nil {
		return err
	}
	env := []string{"CODEX_HOME=" + home}
	for _, kv := range environ {
		if !strings.HasPrefix(kv, "CODEX") && !strings.HasPrefix(kv, "CLAUDE") {
			env = append(env, kv)
		}
	}
	argv := append(append([]string{mock, "exec"}, strings.Fields(rec.Setup["cmdflags"])...), "--script", script)
	argv = append(append(argv, words...), rec.Prompt)
	res, err := procexec.Run(context.Background(), procexec.Spec{Argv: argv, Dir: repo, Env: env})
	switch {
	case err != nil:
		return err
	case res.ExitCode == 0:
		return &core.MockFailure{Detail: fmt.Sprintf("the mock ran with %s, which it must refuse", flag)}
	case !strings.Contains(string(res.Stderr), flag):
		return &core.MockFailure{Detail: fmt.Sprintf("the mock refused (exit %d) without naming %s: %s", res.ExitCode, flag, res.Stderr)}
	}
	if _, err := os.Stat(ran); err == nil {
		return &core.MockFailure{Detail: "the mock refused " + flag + " but ran the script"}
	}
	return nil
}
