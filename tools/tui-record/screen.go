package main

import (
	"strings"
	"sync"
	"unicode/utf8"
)

// Screen is what the terminal has shown so far as plain text: every escape sequence dropped and
// every run of whitespace collapsed to one space, so a match does not depend on how the program
// laid out or redrew its screen. A terminal query the program makes is answered through reply
// (a program that waits for the answer would otherwise stall, as no terminal is there to give it).
//
// The text only grows: a redraw adds its text again. A wait therefore matches from a mark, the
// end of the previous step's match, never from the start.
type Screen struct {
	mu    sync.Mutex
	text  strings.Builder
	state int    // 0 text, 1 after ESC, 2 in a CSI, 3 in an OSC or another string, 4 after ESC inside one
	csi   []byte // the CSI sequence so far
	part  []byte // the bytes of a UTF-8 character the chunk ended in
	reply func(string)
	// changed is closed and replaced each time text is added, so a waiter sleeps until it can match.
	changed chan struct{}
}

// NewScreen makes an empty screen; reply gets the answer to a terminal query.
func NewScreen(reply func(string)) *Screen {
	return &Screen{reply: reply, changed: make(chan struct{})}
}

// Write takes a chunk of the program's output.
func (s *Screen) Write(p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	before := s.text.Len()
	data := append(append([]byte{}, s.part...), p...)
	s.part = nil
	for _, b := range data {
		s.feed(b)
	}
	if s.text.Len() != before {
		close(s.changed)
		s.changed = make(chan struct{})
	}
}

func (s *Screen) feed(b byte) {
	switch s.state {
	case 1:
		s.state = 0
		switch b {
		case '[':
			s.state, s.csi = 2, s.csi[:0]
		case ']', 'P', '_', '^', 'X':
			s.state = 3
		}
	case 2:
		s.csi = append(s.csi, b)
		if b >= 0x40 && b <= 0x7e {
			s.state = 0
			s.answer(string(s.csi))
			s.mode(s.csi)
		}
	case 3:
		if b == 0x07 {
			s.state = 0
		} else if b == 0x1b {
			s.state = 4
		}
	case 4:
		s.state = 3
		if b == '\\' {
			s.state = 0
		}
	default:
		s.text0(b)
	}
}

func (s *Screen) text0(b byte) {
	switch {
	case b == 0x1b:
		s.state = 1
	case b == ' ' || b == '\n' || b == '\r' || b == '\t':
		if t := s.text.String(); t != "" && !strings.HasSuffix(t, " ") {
			s.text.WriteByte(' ')
		}
	case b < 0x20 || b == 0x7f:
	case b < utf8.RuneSelf:
		s.text.WriteByte(b)
	default: // a UTF-8 character may be cut by the chunk: keep its bytes until it is whole
		s.part = append(s.part, b)
		if utf8.FullRune(s.part) {
			s.text.Write(s.part)
			s.part = nil
		}
	}
}

// mode puts a private terminal mode a program sets (CSI ? 2004 h: bracketed paste on) into the
// text as a token, <?2004h>, so a script can wait for it: a TUI turns the modes on once its input
// is live, and what is typed before that is lost, though its screen is already drawn.
func (s *Screen) mode(csi []byte) {
	if n := len(csi); n > 2 && csi[0] == '?' && (csi[n-1] == 'h' || csi[n-1] == 'l') {
		s.text0(' ')
		s.text.WriteString("<" + string(csi) + ">")
		s.text0(' ')
	}
}

// answer replies to the queries a TUI makes of its terminal: the cursor position and the device
// attributes. Anything else a CSI says is a drawing instruction.
func (s *Screen) answer(csi string) {
	switch csi {
	case "6n":
		s.reply("\x1b[1;1R")
	case "5n":
		s.reply("\x1b[0n")
	case "c", "0c":
		s.reply("\x1b[?62;c")
	}
}

// Len is how much text there is, to take a mark.
func (s *Screen) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.text.Len()
}

// Since is the text from a mark, and a channel closed when more arrives.
func (s *Screen) Since(mark int) (string, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.text.String()[mark:], s.changed
}

// Tail is the last n bytes of text, for a failure to show where the run was.
func (s *Screen) Tail(n int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.text.String()
	if len(t) > n {
		t = t[len(t)-n:]
	}
	return t
}
