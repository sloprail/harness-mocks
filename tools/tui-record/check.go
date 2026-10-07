package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// check validates the script and fills its defaults, and compiles every regexp once.
func (s *Script) check(dir string) error {
	if s.Timeout == 0 {
		s.Timeout = 5 * time.Minute
	}
	if s.ExitTimeout == 0 {
		s.ExitTimeout = 20 * time.Second
	}
	if s.Cols == 0 || s.Rows == 0 {
		s.Cols, s.Rows = 160, 50
	}
	for i := range s.Handlers {
		h := &s.Handlers[i]
		if h.Send == nil || h.Screen == "" {
			return fmt.Errorf("handler %d: needs screen and send", i+1)
		}
		var err error
		if h.re, err = regexp.Compile(h.Screen); err != nil {
			return fmt.Errorf("handler %d: %w", i+1, err)
		}
		if err := h.Send.check(dir); err != nil {
			return fmt.Errorf("handler %d: %w", i+1, err)
		}
	}
	for _, list := range [][]Step{s.Steps, s.Exit} {
		for i := range list {
			if err := list[i].check(dir); err != nil {
				return fmt.Errorf("step %d: %w", i+1, err)
			}
		}
	}
	return nil
}

func (st *Step) check(dir string) error {
	if (st.Wait == nil) == (st.Send == nil) {
		return fmt.Errorf("a step is exactly one of wait and send")
	}
	if st.Send != nil {
		return st.Send.check(dir)
	}
	return st.Wait.check()
}

func (w *Wait) check() error {
	if (w.Screen == "") == (w.Hook == "") {
		return fmt.Errorf("a wait is exactly one of screen and hook")
	}
	if w.Timeout == 0 {
		w.Timeout = 60 * time.Second
	}
	if w.Nth == 0 {
		w.Nth = 1
	}
	var err error
	if w.Screen != "" {
		w.re, err = regexp.Compile(w.Screen)
	}
	return err
}

func (s *Send) check(dir string) error {
	if s.TextFile != "" {
		b, err := os.ReadFile(filepath.Join(dir, s.TextFile))
		if err != nil {
			return err
		}
		s.Text = strings.TrimSuffix(string(b), "\n")
	}
	if s.Text == "" && len(s.Keys) == 0 {
		return fmt.Errorf("a send has text or keys")
	}
	for _, k := range s.Keys {
		if _, ok := keyBytes[k]; !ok {
			return fmt.Errorf("unknown key %q", k)
		}
	}
	var err error
	if s.Echo == "" {
		s.echo, err = regexp.Compile(regexp.QuoteMeta(lastChars(strings.Join(strings.Fields(s.Text), " "), 16)))
	} else {
		s.echo, err = regexp.Compile(s.Echo)
	}
	return err
}

func lastChars(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}
