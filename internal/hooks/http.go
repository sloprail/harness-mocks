package hooks

import "strings"

// HTTPOutcome is how a hook that is an HTTP endpoint ended.
type HTTPOutcome int

const (
	// HTTPAccepted: a 2xx with no body, like a command that printed nothing and
	// exited 0.
	HTTPAccepted HTTPOutcome = iota
	// HTTPDecided: a 2xx whose body is a JSON object, read like a command's
	// printed JSON (and a non-blocking error if it fails the output schema).
	HTTPDecided
	// HTTPNonBlockingError: the call failed or the answer cannot be read; the
	// event carries on. No status alone blocks.
	HTTPNonBlockingError
	// HTTPCancelled: the timeout stopped it.
	HTTPCancelled
)

// HTTPResult is how an HTTP hook ended: reached is whether the endpoint was
// reached at all, status its response status and body what it answered.
func HTTPResult(reached, timedOut bool, status int, body string) HTTPOutcome {
	if timedOut {
		return HTTPCancelled
	}
	if !reached || status < 200 || status > 299 {
		return HTTPNonBlockingError
	}
	body = strings.TrimSpace(body)
	switch {
	case body == "":
		return HTTPAccepted
	case strings.HasPrefix(body, "{") && strings.HasSuffix(body, "}"):
		return HTTPDecided
	}
	return HTTPNonBlockingError
}
