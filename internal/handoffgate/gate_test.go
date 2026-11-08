package handoffgate

import (
	"context"
	"testing"
	"time"
)

func reset(t *testing.T) {
	t.Helper()
	Open()
	t.Cleanup(Open)
}

func TestWaitPassesWhenNotArmed(t *testing.T) {
	reset(t)
	if !Wait(t.Context()) {
		t.Fatal("Wait() = false on an unarmed gate")
	}
}

func TestWaitBlocksUntilOpen(t *testing.T) {
	reset(t)
	Arm()
	got := make(chan bool, 1)
	go func() { got <- Wait(t.Context()) }()
	select {
	case <-got:
		t.Fatal("Wait() returned while the gate was closed")
	case <-time.After(50 * time.Millisecond):
	}
	Open()
	select {
	case ok := <-got:
		if !ok {
			t.Fatal("Wait() = false after Open")
		}
	case <-time.After(time.Second):
		t.Fatal("Wait() did not return after Open")
	}
}

func TestWaitStopsOnCancel(t *testing.T) {
	reset(t)
	Arm()
	ctx, cancel := context.WithCancel(t.Context())
	got := make(chan bool, 1)
	go func() { got <- Wait(ctx) }()
	cancel()
	select {
	case ok := <-got:
		if ok {
			t.Fatal("Wait() = true for a cancelled context")
		}
	case <-time.After(time.Second):
		t.Fatal("Wait() ignored the cancelled context")
	}
}

func TestStartupRunsAtOnceWhenNotArmed(t *testing.T) {
	reset(t)
	ran := false
	if err := Startup(t.Context(), "x", func() error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("Startup() ran=%v err=%v, want it to run synchronously", ran, err)
	}
}

func TestStartupWaitsForTheGateThenRuns(t *testing.T) {
	reset(t)
	Arm()
	started := make(chan struct{})
	if err := Startup(t.Context(), "x", func() error { close(started); return nil }); err != nil {
		t.Fatalf("Startup() error = %v", err)
	}
	select {
	case <-started:
		t.Fatal("the startup ran while the gate was closed")
	case <-time.After(50 * time.Millisecond):
	}
	Open()
	if err := Settled(t.Context()); err != nil {
		t.Fatalf("Settled() error = %v", err)
	}
	select {
	case <-started:
	default:
		t.Fatal("the startup did not run after Open")
	}
}

func TestStartupSkippedWhenTheContextEndsFirst(t *testing.T) {
	reset(t)
	Arm()
	ctx, cancel := context.WithCancel(t.Context())
	if err := Startup(ctx, "x", func() error { t.Error("ran after the context ended"); return nil }); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := Settled(t.Context()); err != nil {
		t.Fatalf("Settled() error = %v", err)
	}
	Open()
}
