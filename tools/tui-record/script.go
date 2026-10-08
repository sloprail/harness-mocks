package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"go.yaml.in/yaml/v3"
)

// Script is a declarative drive of an interactive run: what to wait for on the screen or in the
// hook log, and what to type after it. Nothing waits for a fixed time: every wait has a bound.
type Script struct {
	// Timeout bounds the whole run (default 5m).
	Timeout time.Duration `yaml:"timeout"`
	Cols    uint16        `yaml:"cols"`
	Rows    uint16        `yaml:"rows"`
	// Handlers answer a screen text whenever it appears, wherever the run is: a startup dialog
	// that shows up on one start and not another.
	Handlers []Handler `yaml:"handlers"`
	Steps    []Step    `yaml:"steps"`
	// Exit are the steps that end the session; the process is then waited for (ExitTimeout,
	// default 20s) and killed if it is still there.
	Exit        []Step        `yaml:"exit"`
	ExitTimeout time.Duration `yaml:"exit_timeout"`
}

// Handler sends Send each time Screen newly appears.
type Handler struct {
	Screen string `yaml:"screen"`
	Send   *Send  `yaml:"send"`
	re     *regexp.Regexp
}

// Step is one wait or one send.
type Step struct {
	Wait *Wait `yaml:"wait"`
	Send *Send `yaml:"send"`
}

// Wait ends when a screen regexp appears in the output since the previous step's match, or when
// a hook event has been logged Nth times (default 1) in the hook log, or the Timeout (default
// 60s) passes, which fails the run.
type Wait struct {
	Screen  string        `yaml:"screen"`
	Hook    string        `yaml:"hook"`
	Nth     int           `yaml:"nth"`
	Timeout time.Duration `yaml:"timeout"`
	re      *regexp.Regexp
}

// Send types Text, then waits for the program to have drawn it (Echo, a screen regexp; by default
// the end of the text itself) so the keys after it are not taken as part of one paste, then
// presses Keys (names: enter, esc, tab, ctrl-c, up, ...). A terminal's own echo of the typed
// characters comes before the program has taken them: an Echo that only the program's own
// drawing shows (its input line's marker, say) is the proof it has.
type Send struct {
	Text string `yaml:"text"`
	// TextFile is a file, beside the script, whose content (less its last newline) is the text:
	// a prompt the setup already holds as prompt.txt is not written twice.
	TextFile string   `yaml:"text_file"`
	Echo     string   `yaml:"echo"`
	Keys     []string `yaml:"keys"`
	echo     *regexp.Regexp
}

// Load reads and checks a script file.
func Load(path string) (*Script, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Script
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if err := d.Decode(&s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := s.check(filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &s, nil
}
