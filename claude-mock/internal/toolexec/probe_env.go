package toolexec

import "os"

// probeEnv reads the environment outside the entrypoint (rule probe).
func probeEnv() string { return os.Getenv("PROBE") }
