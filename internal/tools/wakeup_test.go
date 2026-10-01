package tools

import (
	"errors"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }

func id() string { return "w1" }

var noon = time.Date(2026, 10, 1, 12, 0, 20, 0, time.UTC)

func TestSchedule_ClampsTheDelayAndRoundsUpToAMinute(t *testing.T) {
	for _, tc := range []struct {
		delay   int
		want    time.Time
		seconds int
		clamped bool
	}{
		{120, time.Date(2026, 10, 1, 12, 3, 0, 0, time.UTC), 160, false},
		{10, time.Date(2026, 10, 1, 12, 2, 0, 0, time.UTC), 100, true},
		{100000, time.Date(2026, 10, 1, 13, 1, 0, 0, time.UTC), 3640, true},
		{-5, time.Date(2026, 10, 1, 12, 2, 0, 0, time.UTC), 100, true},
	} {
		w := NewWakeups()
		got, err := w.Schedule(WakeupRequest{DelaySeconds: ptr(tc.delay), Prompt: ptr("p"), Noop: ptr(false)}, noon, id)
		if err != nil {
			t.Fatal(err)
		}
		if !got.At.Equal(tc.want) || got.SecondsUntil(noon) != tc.seconds || got.Clamped != tc.clamped {
			t.Fatalf("delay %d: %+v (in %ds), want %v in %ds clamped=%v", tc.delay, got, got.SecondsUntil(noon), tc.want, tc.seconds, tc.clamped)
		}
	}
}

func TestSchedule_AnExactMinuteIsNotRoundedFurther(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	got, _ := NewWakeups().Schedule(WakeupRequest{DelaySeconds: ptr(120), Prompt: ptr("p"), Noop: ptr(false)}, now, id)
	if !got.At.Equal(now.Add(2 * time.Minute)) {
		t.Fatalf("At = %v", got.At)
	}
}

func TestSchedule_MissingArgumentsAreErrors(t *testing.T) {
	for want, req := range map[string]WakeupRequest{
		"delaySeconds": {Prompt: ptr("p"), Noop: ptr(false)},
		"prompt":       {DelaySeconds: ptr(60), Noop: ptr(false)},
		"noop":         {DelaySeconds: ptr(60), Prompt: ptr("p")},
	} {
		_, err := NewWakeups().Schedule(req, noon, id)
		var miss *MissingArgError
		if !errors.As(err, &miss) || miss.Arg != want {
			t.Fatalf("err = %v, want missing %s", err, want)
		}
	}
	if _, err := NewWakeups().Schedule(WakeupRequest{DelaySeconds: ptr(60), Prompt: ptr(""), Noop: ptr(false)}, noon, id); err == nil {
		t.Fatal("an empty prompt is missing")
	}
}

func TestSchedule_OnePendingAndStopCancelsIt(t *testing.T) {
	w := NewWakeups()
	if r, _ := w.Schedule(WakeupRequest{Stop: true}, noon, id); !r.Stopped || r.Cancelled != 0 {
		t.Fatalf("stop with nothing pending = %+v", r)
	}
	w.Schedule(WakeupRequest{DelaySeconds: ptr(60), Prompt: ptr("first"), Noop: ptr(false)}, noon, id)
	w.Schedule(WakeupRequest{DelaySeconds: ptr(120), Prompt: ptr("second"), Noop: ptr(false)}, noon, func() string { return "w2" })
	if p := w.Pending(); len(p) != 1 || p[0].Prompt != "second" || p[0].ID != "w2" {
		t.Fatalf("pending = %+v: a new request replaces the last", p)
	}
	if r, _ := w.Schedule(WakeupRequest{Stop: true}, noon, id); !r.Stopped || r.Cancelled != 1 {
		t.Fatalf("stop = %+v", r)
	}
	if len(w.Pending()) != 0 {
		t.Fatal("still pending after stop")
	}
	var nilW *Wakeups
	if nilW.Pending() != nil {
		t.Fatal("nil session has pending wake-ups")
	}
}
