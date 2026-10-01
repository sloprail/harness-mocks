package hooks

import (
	"context"
	"testing"
	"time"
)

func invoke(line string, strict bool) Run {
	return Invoke(context.Background(), Command{Line: line, Stdin: []byte("payload"), Env: []string{"PATH=/usr/bin:/bin", "V=7"}, Strict: strict})
}

func TestInvokeReadsStdinOutputAndVerdict(t *testing.T) {
	r := invoke(`read p; echo "out $p $V"; echo err >&2; exit 2`, false)
	if r.Stdout != "out payload 7\n" || r.Stderr != "err\n" || r.ExitCode != 2 || r.Verdict != Blocked {
		t.Fatalf("Invoke = %+v", r)
	}
	if r := invoke("exit 0", false); r.Verdict != Accepted {
		t.Errorf("exit 0 = %+v", r)
	}
	if r := invoke("exit 3", false); r.Verdict != NonBlockingError || r.ExitCode != 3 {
		t.Errorf("exit 3 = %+v", r)
	}
	if r := invoke("exit 3", true); r.Verdict != Blocked {
		t.Errorf("a strict event takes exit 3 as blocking: %+v", r)
	}
}

func TestInvokeUnstartableAndKilledAreNonBlockingErrors(t *testing.T) {
	r := Invoke(context.Background(), Command{Line: "sleep 5", Env: []string{"PATH=/usr/bin:/bin"}, Timeout: 100 * time.Millisecond})
	if r.Verdict != NonBlockingError || r.ExitCode != -1 {
		t.Fatalf("a killed command = %+v, want a non-blocking error with no exit status", r)
	}
	r = Invoke(context.Background(), Command{Line: "/nonexistent/hook", Env: []string{"PATH=/usr/bin:/bin"}})
	if r.Verdict != NonBlockingError || r.ExitCode != 127 {
		t.Fatalf("a missing program = %+v, want the shell's 127, a non-blocking error", r)
	}
}
