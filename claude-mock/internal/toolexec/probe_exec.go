package toolexec

import "os/exec"

// probeExec spawns a child outside procexec (rule probe).
func probeExec() error { return exec.Command("true").Run() }
