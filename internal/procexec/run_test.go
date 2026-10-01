package procexec

import (
	"context"
	"strings"
	"testing"
	"time"
)

func sh(script string) Spec {
	return Spec{Argv: []string{"/bin/sh", "-c", script}, Env: []string{"PATH=/usr/bin:/bin", "X=1"}}
}

func TestRunReportsOutputStdinAndExit(t *testing.T) {
	s := sh(`read l; echo "got $l $X"; echo err >&2; exit 3`)
	s.Stdin = []byte("line\n")
	res, err := Run(context.Background(), s)
	if err != nil || !res.Started {
		t.Fatalf("Run = %+v, %v", res, err)
	}
	if res.ExitCode != 3 || string(res.Stdout) != "got line 1\n" || string(res.Stderr) != "err\n" {
		t.Fatalf("result = %+v", res)
	}
}

func TestRunUnstartableProgramIsAnError(t *testing.T) {
	res, err := Run(context.Background(), Spec{Argv: []string{"/nonexistent/program"}})
	if err == nil || res.Started || res.ExitCode != -1 {
		t.Fatalf("Run = %+v, %v; want an unstarted error", res, err)
	}
}

func TestRunTimeoutKillsTheWholeGroup(t *testing.T) {
	s := sh(`sleep 30 & echo $!; wait`)
	s.Timeout = 300 * time.Millisecond
	start := time.Now()
	res, err := Run(context.Background(), s)
	if err != nil || !res.TimedOut || res.ExitCode != -1 {
		t.Fatalf("Run = %+v, %v; want a timeout", res, err)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("the timeout took %v", time.Since(start))
	}
	pid := strings.TrimSpace(string(res.Stdout))
	probe, _ := Run(context.Background(), sh(`kill -0 `+pid+` 2>/dev/null && echo alive || echo gone`))
	if got := strings.TrimSpace(string(probe.Stdout)); got != "gone" {
		t.Fatalf("grandchild %s is %s after the timeout", pid, got)
	}
}
