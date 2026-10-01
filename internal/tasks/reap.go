package tasks

import "time"

// stopOwned kills the commands owner still has running, marks them handed over
// (a killed command is reported on the stream only) and waits for them to be
// gone. Agents are not killed.
func (r *Registry) stopOwned(owner string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	var victims []*Task
	for _, t := range r.tasks {
		if t.Owner == owner && t.Kind == Command && !t.Finished() {
			t.killed.Store(true)
			t.delivered = true
			victims = append(victims, t)
		}
	}
	r.mu.Unlock()
	for _, t := range victims {
		t.kill()
		<-t.done
	}
}

// EndOfResponse ends what a foreground sub-agent still has running when it
// gives its final response: its background commands are terminated.
//
// sr:capability foreground-subagent-bash-ends-with-response
func (r *Registry) EndOfResponse(owner string) { r.stopOwned(owner) }

// ReapAtExit ends what owner still has running when a non-interactive run's
// other work is done: its background commands are terminated, once grace has
// passed, so that a command that finishes right after the final result still
// delivers its output.
//
// sr:capability background-bash-reaped-at-exit
func (r *Registry) ReapAtExit(owner string, grace time.Duration) {
	if r == nil {
		return
	}
	r.mu.Lock()
	var running []*Task
	for _, t := range r.tasks {
		if t.Owner == owner && t.Kind == Command && !t.Finished() {
			running = append(running, t)
		}
	}
	r.mu.Unlock()
	timeout := time.After(grace)
	for _, t := range running {
		select {
		case <-t.done:
		case <-timeout:
			r.stopOwned(owner)
			return
		}
	}
}

// Shutdown ends the whole session's background work when the run returns: it
// kills every running command, cancels the registry's context (and so running
// background agents) and waits for all of it.
func (r *Registry) Shutdown() {
	if r == nil {
		return
	}
	r.mu.Lock()
	var kills []func()
	for _, t := range r.tasks {
		if t.Kind == Command && !t.Finished() {
			t.killed.Store(true)
			kills = append(kills, t.kill)
		}
	}
	r.mu.Unlock()
	for _, kill := range kills {
		kill()
	}
	r.cancel()
	r.wg.Wait()
}
