//go:build devmock && !windows

package procs

import (
	"context"
	"testing"

	"typhon/internal/devmock"
)

func TestListReturnsDevmockEntries(t *testing.T) {
	t.Setenv(devmock.StateDirEnv, t.TempDir())

	started, err := devmock.Start("/games/demo/demo.exe", nil, "/games/demo")
	if err != nil {
		t.Fatalf("devmock.Start: %v", err)
	}
	t.Cleanup(func() {
		if err := started.Kill(); err != nil {
			t.Fatalf("cleanup kill: %v", err)
		}
	})

	got, err := List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var found *Process
	for i := range got {
		if got[i].PID == started.PID() {
			found = &got[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("List() = %+v, want an entry with pid %d", got, started.PID())
	}
	if found.Path != started.Path() {
		t.Fatalf("Path = %q, want %q", found.Path, started.Path())
	}
	if found.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero")
	}
	if found.PathUnknown || found.CreatedAtUnknown {
		t.Fatalf("PathUnknown=%v CreatedAtUnknown=%v, want both false", found.PathUnknown, found.CreatedAtUnknown)
	}
}

func TestListCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := List(ctx)
	if err == nil {
		t.Fatal("List with cancelled context: expected error")
	}
	if got != nil {
		t.Fatalf("List with cancelled context = %v, want nil", got)
	}
}
