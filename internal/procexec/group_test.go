package procexec

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestKillGroupReachesChildren(t *testing.T) {
	// The shell's child holds the stdout pipe open; Wait returns only once the
	// whole group is gone.
	cmd := exec.Command("/bin/sh", "-c", "sleep 30 & sleep 30; wait")
	OwnGroup(cmd)
	r, w, _ := os.Pipe()
	cmd.Stdout = w
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	w.Close()
	if err := KillGroup(cmd); err != nil {
		t.Fatalf("KillGroup: %v", err)
	}
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 1)
		_, _ = r.Read(buf) // EOF once every writer, the grandchildren too, is dead
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the children outlived their group")
	}
}

func TestKillGroupOfGoneProcess(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	OwnGroup(cmd)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if err := KillGroup(cmd); !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("KillGroup of a finished command = %v, want os.ErrProcessDone", err)
	}
	if err := KillGroup(nil); !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("KillGroup(nil) = %v", err)
	}
}
