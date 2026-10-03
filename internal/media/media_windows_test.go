package media

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

func threadID(context.Context) (uint32, error) {
	return windows.GetCurrentThreadId(), nil
}

func TestApartmentRunsEverythingOnOneThread(t *testing.T) {
	var released atomic.Int32
	a := newApartment(func() { released.Add(1) })
	if err := a.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	first, err := runOn(context.Background(), a, threadID)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	const callers = 8
	var wg sync.WaitGroup
	ids := make([]uint32, callers)
	errs := make([]error, callers)
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids[i], errs[i] = runOn(context.Background(), a, threadID)
		}()
	}
	wg.Wait()
	for i := range callers {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}
		if ids[i] != first {
			t.Fatalf("caller %d ran on thread %d, want %d", i, ids[i], first)
		}
	}

	if err := a.stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if released.Load() != 1 {
		t.Fatalf("cleanup ran %d times, want 1", released.Load())
	}
}

func TestApartmentRefusesWorkWhenNotRunning(t *testing.T) {
	a := newApartment(func() {})
	if _, err := runOn(context.Background(), a, threadID); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("before start: error = %v, want ErrNotStarted", err)
	}
	if err := a.stop(); err != nil {
		t.Fatalf("stop before start: %v", err)
	}

	if err := a.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := a.start(context.Background()); err == nil {
		t.Fatal("a second start was accepted")
	}
	if err := a.stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := a.stop(); err != nil {
		t.Fatalf("second stop: %v", err)
	}
	if _, err := runOn(context.Background(), a, threadID); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("after stop: error = %v, want ErrNotStarted", err)
	}
}

func TestApartmentStopsWhenTheParentContextEnds(t *testing.T) {
	a := newApartment(func() {})
	ctx, cancel := context.WithCancel(context.Background())
	if err := a.start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	cancel()
	<-a.stopped
	if _, err := runOn(context.Background(), a, threadID); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("after the parent context ended: error = %v, want ErrNotStarted", err)
	}
	if err := a.stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

func TestApartmentCallerContext(t *testing.T) {
	a := newApartment(func() {})
	if err := a.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		if err := a.stop(); err != nil {
			t.Errorf("stop: %v", err)
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runOn(ctx, a, threadID); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want Canceled", err)
	}

	boom := errors.New("boom")
	if _, err := runOn(context.Background(), a, func(context.Context) (int, error) { return 7, boom }); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to be %v", err, boom)
	}
}

func TestServiceBeforeStartup(t *testing.T) {
	svc, err := NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if _, err := svc.Current(context.Background()); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("Current error = %v, want ErrNotStarted", err)
	}
	if err := svc.TogglePlayPause(context.Background()); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("TogglePlayPause error = %v, want ErrNotStarted", err)
	}
}

func TestServiceStartupAndShutdown(t *testing.T) {
	svc, err := NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	if err := svc.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
	if _, err := svc.Current(context.Background()); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("Current after shutdown: error = %v, want ErrNotStarted", err)
	}
}

func TestCommandSlot(t *testing.T) {
	tests := []struct {
		cmd  command
		slot uintptr
	}{
		{cmdTogglePlayPause, slotTryTogglePlayPause},
		{cmdNext, slotTrySkipNext},
		{cmdPrevious, slotTrySkipPrevious},
	}
	for _, tt := range tests {
		slot, _, err := commandSlot(tt.cmd)
		if err != nil || slot != tt.slot {
			t.Errorf("commandSlot(%d) = %d, %v; want %d", tt.cmd, slot, err, tt.slot)
		}
	}
	if _, _, err := commandSlot(command(99)); err == nil {
		t.Error("an unknown command was accepted")
	}
}

func TestApartmentAbortsARunningJobWhenTheCallerGivesUp(t *testing.T) {
	a := newApartment(func() {})
	if err := a.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		if err := a.stop(); err != nil {
			t.Errorf("stop: %v", err)
		}
	})

	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := runOn(ctx, a, func(ctx context.Context) (int, error) {
			close(started)
			<-ctx.Done()
			return 0, ctx.Err()
		})
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want Canceled", err)
	}

	if _, err := runOn(context.Background(), a, threadID); err != nil {
		t.Fatalf("the worker stayed blocked on the abandoned job: %v", err)
	}
}

func TestApartmentStopAbortsARunningJob(t *testing.T) {
	a := newApartment(func() {})
	if err := a.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}

	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := runOn(context.Background(), a, func(ctx context.Context) (int, error) {
			close(started)
			<-ctx.Done()
			return 0, ctx.Err()
		})
		done <- err
	}()
	<-started
	if err := a.stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want Canceled", err)
	}
}
