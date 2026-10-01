package hooks

import "time"

// retryDelays is how long to wait before each retry of a hook command that
// failed: a command that fails is tried again three times, backing off.
var retryDelays = []time.Duration{100 * time.Millisecond, 400 * time.Millisecond, 1600 * time.Millisecond}

// withRetry runs fn until it succeeds or the delays run out.
func withRetry(fn func() error) error {
	err := fn()
	for _, d := range retryDelays {
		if err == nil {
			return nil
		}
		time.Sleep(d)
		err = fn()
	}
	return err
}
