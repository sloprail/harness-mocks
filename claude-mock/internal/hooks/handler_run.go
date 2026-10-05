package hooks

// HandlerRun is what one hook handler did, in the terms real Claude Code
// records it in a transcript's hook attachment: its command, its streams, its
// exit code and how long it took. Blocked is an exit 2.
type HandlerRun struct {
	Command    string
	Stdout     string
	Stderr     string
	ExitCode   int
	DurationMs int64
	Blocked    bool
	Output     Output
	// JSONParsed: stdout was a JSON object the mock read; on a non-blocking
	// exit status it then decides, not the status (docs, "Other exit codes").
	// JSONError: stdout looked like JSON but did not parse or validate.
	JSONParsed bool
	JSONError  string
	// BlockReason is the blocking reason a blocked handler's JSON gave.
	BlockReason string
	// TimedOut: its timeout cancelled it (TimeoutMs is the limit); its output
	// is discarded.
	TimedOut  bool
	TimeoutMs int64
	// HTTPError: an HTTP hook that failed or answered what cannot be read,
	// a non-blocking error.
	HTTPError string
}
