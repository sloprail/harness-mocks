package tasks

import "time"

// stopOwned kills the commands owner still has running, marks them handed over
// (a killed command is reported on the stream only) and waits for them to be
// gone. Agents are not killed.
func (r *Registry) stopOwned(owner string) []*Task {
	if r == nil {
		return nil
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
	return victims
}

// EndOfResponse ends what a foreground sub-agent still has running when it
// gives its final response: its background commands are terminated. It returns
// the commands it ended, for a harness whose stream reports their end (when it
// does is the harness's parameter: see Deferred).
//
// sr:capability foreground-subagent-bash-ends-with-response
func (r *Registry) EndOfResponse(owner string) []*Task { return r.stopOwned(owner) }

// Deferred holds the stream frames that report what ended at a sub-agent's final
// response until the harness's stream is owed them: Cursor reports them after
// the parent's next tool call, or at the end of the run (recorded:
// runs/foreground-subagent-bash-ends-with-response).
type Deferred struct{ frames [][]byte }

// Hold keeps a frame until Release.
func (d *Deferred) Hold(frame []byte) { d.frames = append(d.frames, frame) }

// Release hands the held frames, in the order held, to emit and forgets them.
func (d *Deferred) Release(emit func([]byte)) {
	held := d.frames
	d.frames = nil
	for _, f := range held {
		emit(f)
	}
}

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
