package toolexec

import "os"

func probeHome() string { return os.Getenv("HOME") }
