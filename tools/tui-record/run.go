package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// Config is one recording's surroundings; everything of it is the caller's, nothing is looked up.
type Config struct {
	Bin            string
	Args           []string
	Dir, Home, Tmp string
	HookLog        string
	Env            []string
	Raw            io.Writer // the program's raw output, if the caller wants it
	Log            io.Writer // one JSON line per step done, and the end
}

// Result is how the session ended: the program's exit status, or Killed when it had to be
// killed (its status then says nothing).
type Result struct {
	Exit   int
	Killed bool
}

type driver struct {
	cfg    *Config
	sc     *Script
	ptmx   *os.File
	wmu    sync.Mutex
	screen *Screen
	mark   int
	done   chan struct{} // closed when the process has exited
	n      int
}

// Run starts the pinned binary on a pseudo-terminal and plays the script against it.
func Run(cfg *Config, sc *Script) (Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sc.Timeout)
	defer cancel()
	cmd := exec.Command(cfg.Bin, cfg.Args...)
	cmd.Dir, cmd.Env = cfg.Dir, environment(cfg)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: sc.Rows, Cols: sc.Cols})
	if err != nil {
		return Result{}, err
	}
	d := &driver{cfg: cfg, sc: sc, ptmx: ptmx, done: make(chan struct{})}
	d.screen = NewScreen(d.write)
	var status error
	go func() { status = cmd.Wait(); close(d.done) }()
	go d.read()
	err = d.play(ctx)
	res := Result{}
	if err == nil {
		select {
		case <-d.done:
		case <-time.After(sc.ExitTimeout):
			res.Killed = true
		case <-ctx.Done():
			res.Killed = true
		}
	}
	if res.Killed || err != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-d.done
		res.Killed = true
	}
	_ = ptmx.Close()
	var ee *exec.ExitError
	if errors.As(status, &ee) {
		res.Exit = ee.ExitCode()
	}
	d.log(map[string]any{"end": true, "killed": res.Killed})
	if err != nil {
		return res, fmt.Errorf("%w\n--- the screen's last text ---\n%s", err, d.screen.Tail(1500))
	}
	return res, nil
}

func (d *driver) write(s string) {
	d.wmu.Lock()
	defer d.wmu.Unlock()
	_, _ = d.ptmx.WriteString(s)
}

// read feeds the program's output to the screen, and lets the handlers answer what they match.
func (d *driver) read() {
	marks := make([]int, len(d.sc.Handlers))
	buf := make([]byte, 32<<10)
	for {
		n, err := d.ptmx.Read(buf)
		if n > 0 {
			if d.cfg.Raw != nil {
				_, _ = d.cfg.Raw.Write(buf[:n])
			}
			d.screen.Write(buf[:n])
			for i, h := range d.sc.Handlers {
				text, _ := d.screen.Since(marks[i])
				if loc := h.re.FindStringIndex(text); loc != nil {
					marks[i] += loc[1]
					d.press(h.Send)
					d.log(map[string]any{"handler": h.Screen})
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func (d *driver) press(s *Send) {
	d.write(s.Text)
	for _, k := range s.Keys {
		d.write(keyBytes[k])
	}
}

func (d *driver) log(v map[string]any) {
	if d.cfg.Log != nil {
		b, _ := json.Marshal(v)
		_, _ = d.cfg.Log.Write(append(b, '\n'))
	}
}
