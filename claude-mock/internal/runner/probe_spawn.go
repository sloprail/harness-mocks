package runner

import "os/exec"

func probeSpawn() error { return exec.Command("true").Run() }
