package procexec

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStartReturnsAtOnceAndWaitReportsTheExit(t *testing.T) {
	var out bytes.Buffer
	started := time.Now()
	p, err := Start(sh(`sleep 0.3; echo out; echo err >&2; exit 4`), &out)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > 200*time.Millisecond {
		t.Fatal("Start waited for the child")
	}
	if code := p.Wait(); code != 4 {
		t.Fatalf("exit = %d", code)
	}
	if got := out.String(); !strings.Contains(got, "out\n") || !strings.Contains(got, "err\n") {
		t.Fatalf("output = %q: stdout and stderr both go to out", got)
	}
}

func TestStartUnstartableAndEmpty(t *testing.T) {
	if _, err := Start(Spec{Argv: []string{"/nonexistent/program"}}, &bytes.Buffer{}); err == nil {
		t.Fatal("want an error")
	}
	if _, err := Start(Spec{}, &bytes.Buffer{}); err == nil {
		t.Fatal("want an error for no program")
	}
}

func TestKillEndsTheWholeGroup(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "child-alive")
	p, err := Start(sh(`(sleep 1.5; touch `+marker+`) & sleep 30`), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	p.Kill()
	if code := p.Wait(); code == 0 {
		t.Fatalf("a killed child exits 0")
	}
	time.Sleep(2 * time.Second)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("the child's own child survived the kill")
	}
	var none *Proc
	none.Kill() // no panic
}
