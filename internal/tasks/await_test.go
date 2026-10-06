package tasks

import (
	"context"
	"testing"
	"time"
)

// Await returns the task once it is registered, woken by the registration and not polling, and nil
// when its context ends first.
func TestAwaitReturnsTheTaskOnceItIsRegistered(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()
	got := make(chan *Task, 1)
	go func() { got <- r.Await(context.Background(), "kid") }()
	select {
	case <-got:
		t.Fatal("returned before the task was registered")
	case <-time.After(50 * time.Millisecond):
	}
	task := NewTask(Agent, "kid")
	r.Add(task)
	select {
	case g := <-got:
		if g != task {
			t.Fatalf("got %v", g)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("not woken by the registration")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r.Await(ctx, "nobody") != nil {
		t.Fatal("a cancelled wait for an unknown task returned one")
	}
}
