package main

import (
	"context"
	"fmt"
	"time"
)

// hookPoll is how often the hook log is read while a wait on it is open: the wait itself ends only
// on the event or its bound.
const hookPoll = 20 * time.Millisecond

func (d *driver) play(ctx context.Context) error {
	for _, list := range [][]Step{d.sc.Steps, d.sc.Exit} {
		for _, st := range list {
			d.n++
			var err error
			if st.Wait != nil {
				err = d.wait(ctx, st.Wait)
			} else {
				err = d.send(ctx, st.Send)
			}
			if err != nil {
				return fmt.Errorf("step %d: %w", d.n, err)
			}
		}
	}
	return nil
}

func (d *driver) wait(ctx context.Context, w *Wait) error {
	ctx, cancel := context.WithTimeout(ctx, w.Timeout)
	defer cancel()
	if w.Hook != "" {
		return d.waitHook(ctx, w)
	}
	for {
		text, changed := d.screen.Since(d.mark)
		if loc := w.re.FindStringIndex(text); loc != nil {
			d.mark += loc[1]
			d.log(map[string]any{"step": d.n, "wait_screen": w.Screen})
			return nil
		}
		select {
		case <-changed:
		case <-d.done:
			return fmt.Errorf("the program exited before the screen showed %q", w.Screen)
		case <-ctx.Done():
			return fmt.Errorf("timed out (%s) waiting for the screen to show %q", w.Timeout, w.Screen)
		}
	}
}

func (d *driver) waitHook(ctx context.Context, w *Wait) error {
	tick := time.NewTicker(hookPoll)
	defer tick.Stop()
	for {
		if countHooks(d.cfg.HookLog, w.Hook) >= w.Nth {
			d.log(map[string]any{"step": d.n, "wait_hook": w.Hook, "nth": w.Nth})
			return nil
		}
		select {
		case <-tick.C:
		case <-d.done:
			return fmt.Errorf("the program exited before hook %s was logged %d time(s)", w.Hook, w.Nth)
		case <-ctx.Done():
			return fmt.Errorf("timed out (%s) waiting for hook %s to be logged %d time(s)", w.Timeout, w.Hook, w.Nth)
		}
	}
}

// send types the text, waits for the screen to echo its end (so the keys after it are not read as
// part of one paste), then presses the keys. The mark moves past what the send itself printed.
func (d *driver) send(ctx context.Context, s *Send) error {
	if s.Text != "" {
		start := d.screen.Len()
		d.write(s.Text)
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		for {
			text, changed := d.screen.Since(start)
			if s.echo.MatchString(text) {
				break
			}
			select {
			case <-changed:
			case <-d.done:
				return fmt.Errorf("the program exited before it drew the text it was sent")
			case <-ctx.Done():
				return fmt.Errorf("the screen never showed %q after the text was typed", s.echo)
			}
		}
	}
	for _, k := range s.Keys {
		d.write(keyBytes[k])
	}
	d.mark = d.screen.Len()
	d.log(map[string]any{"step": d.n, "send_text": s.Text, "send_keys": s.Keys})
	return nil
}
