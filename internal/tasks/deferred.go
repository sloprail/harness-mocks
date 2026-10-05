package tasks

// EndedAtResponse ends what a foreground sub-agent still has running when it
// gives its final response (see EndOfResponse) and returns the commands it ended,
// for a harness whose stream reports their end: when it does is the harness's
// parameter (see Deferred).
func (r *Registry) EndedAtResponse(owner string) []*Task {
	var ending []*Task
	for _, t := range r.Running() {
		if t.Owner == owner && t.Kind == Command {
			ending = append(ending, t)
		}
	}
	r.EndOfResponse(owner)
	return ending
}

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
