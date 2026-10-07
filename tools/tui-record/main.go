// Command tui-record drives an interactive (TUI) run of a real harness on a pseudo-terminal, from a
// declarative script, so every harness's recordings of interactive behaviour are made the same way.
//
//	go run ./tools/tui-record --bin <abs path> --expect-version <v> --script tui.yaml \
//	    --cwd <project> --home <scratch home> --tmp <scratch tmp> --hook-log <file> \
//	    [--arg A]... [--env K=V]... [--link REL=SRC]... [--copy REL=SRC]... \
//	    [--log tui.jsonl] [--exit-file exit.txt] [--raw raw.bin]
//
// The script (see script.go) says what to wait for, on the screen or in the hook log, and what to
// type then; every wait has a bound and nothing sleeps for a fixed time. The binary is the one
// given by absolute path, and must report exactly --expect-version: capture scripts take it from
// tools/harness-bin, never from PATH. The harness starts with an environment of its own (PATH
// only is inherited), a scratch HOME, and only the login --link/--copy put there. The hook
// scripts the project configures append their raw payloads to $HOOK_LOG; that file is the
// recording, and the tool only reads it to know a hook event has happened.
//
// --log gets one JSON line per step done (the structure of the run, free of ids and times) and
// --exit-file the program's exit status ("killed" lines are not an exit). What varies from run
// to run is the model's wording, ids, timestamps and token counts, all inside the payloads.
// Exit status: 0 the script ran to its end, 1 a wait timed out or the program died, 2 usage.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }

func main() { os.Exit(run(os.Args[1:], os.Stderr)) }

func run(args []string, stderr *os.File) int {
	fs := flag.NewFlagSet("tui-record", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var c Config
	var script, version, logPath, exitPath, rawPath string
	var argv, env, links, copies list
	fs.StringVar(&c.Bin, "bin", "", "the pinned binary, an absolute path")
	fs.StringVar(&version, "expect-version", "", "the version it must report")
	fs.StringVar(&script, "script", "", "the script file")
	fs.StringVar(&c.Dir, "cwd", "", "the project the harness starts in")
	fs.StringVar(&c.Home, "home", "", "the scratch HOME")
	fs.StringVar(&c.Tmp, "tmp", "", "the scratch TMPDIR")
	fs.StringVar(&c.HookLog, "hook-log", "", "the file the hook scripts append payloads to")
	fs.StringVar(&logPath, "log", "", "write the steps done here")
	fs.StringVar(&exitPath, "exit-file", "", "write the exit status here")
	fs.StringVar(&rawPath, "raw", "", "write the program's raw output here")
	fs.Var(&argv, "arg", "an argument for the binary (repeatable)")
	fs.Var(&env, "env", "KEY=VALUE for the harness (repeatable)")
	fs.Var(&links, "link", "symlink <path under home>=<source> (repeatable)")
	fs.Var(&copies, "copy", "copy <path under home>=<source file> (repeatable)")
	if fs.Parse(args) != nil {
		return 2
	}
	for _, need := range []struct{ name, v string }{{"bin", c.Bin}, {"expect-version", version}, {"script", script}, {"cwd", c.Dir}, {"home", c.Home}, {"tmp", c.Tmp}, {"hook-log", c.HookLog}} {
		if need.v == "" {
			fmt.Fprintf(stderr, "tui-record: --%s is required\n", need.name)
			return 2
		}
	}
	c.Args, c.Env = argv, env
	sc, err := Load(script)
	if err == nil {
		err = checkVersion(c.Bin, version)
	}
	if err == nil {
		err = prepare(&c, links, copies, logPath, rawPath)
	}
	if err != nil {
		fmt.Fprintln(stderr, "tui-record:", err)
		return 2
	}
	res, err := Run(&c, sc)
	if exitPath != "" && err == nil && !res.Killed {
		_ = os.WriteFile(exitPath, []byte(strconv.Itoa(res.Exit)+"\n"), 0o644)
	}
	if err != nil {
		fmt.Fprintln(stderr, "tui-record:", err)
		return 1
	}
	return 0
}

func prepare(c *Config, links, copies list, logPath, rawPath string) error {
	if err := lay(c.Home, links, true); err != nil {
		return err
	}
	if err := lay(c.Home, copies, false); err != nil {
		return err
	}
	var err error
	if logPath != "" {
		if c.Log, err = os.Create(logPath); err != nil {
			return err
		}
	}
	if rawPath != "" {
		if c.Raw, err = os.Create(rawPath); err != nil {
			return err
		}
	}
	return nil
}
