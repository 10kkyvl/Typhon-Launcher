package install

import (
	"context"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TestNotifyFinishedGoroutineIsTrackedByShutdown closes finding 3:
// notifyFinished span a bare `go notify(item)` with no s.wg.Add and no
// closing check, so ServiceShutdown could return while the callback was
// still running. The goroutine must be in s.wg, and ServiceShutdown must
// block on it.
func TestNotifyFinishedGoroutineIsTrackedByShutdown(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	called := make(chan Installation, 1)
	s.SetOnFinished(func(item Installation) {
		close(entered)
		<-release
		called <- item
	})

	s.notifyFinished(Installation{ID: "a"})
	<-entered

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- s.ServiceShutdown() }()

	select {
	case <-shutdownDone:
		t.Fatal("ServiceShutdown returned before the notify callback finished")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServiceShutdown did not return after the notify callback finished")
	}

	select {
	case item := <-called:
		if item.ID != "a" {
			t.Fatalf("callback item = %+v, want ID=a", item)
		}
	default:
		t.Fatal("onFinished was never invoked")
	}
}

// TestNotifyFinishedSkipsAfterShutdown closes the other half of finding 3:
// once the service is closing, notifyFinished must refuse to start a new
// goroutine at all, matching how spawnLocked and its siblings behave.
func TestNotifyFinishedSkipsAfterShutdown(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}

	called := make(chan struct{}, 1)
	s.SetOnFinished(func(Installation) { called <- struct{}{} })

	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	s.notifyFinished(Installation{ID: "a"})

	select {
	case <-called:
		t.Fatal("onFinished ran after ServiceShutdown")
	case <-time.After(100 * time.Millisecond):
	}
}
