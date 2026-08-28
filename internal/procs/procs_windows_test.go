//go:build windows

package procs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestListFindsCurrentProcess(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	wantPath := filepath.Clean(self)

	list, err := List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	//nolint:gosec // G115: os.Getpid() always fits uint32 on Windows
	pid := uint32(os.Getpid())
	var found *Process
	for i := range list {
		if list[i].PID == pid {
			found = &list[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("List did not contain current process pid %d among %d entries", pid, len(list))
	}
	if found.PathUnknown {
		t.Fatalf("current process PathUnknown = true, want false")
	}
	if found.CreatedAtUnknown {
		t.Fatalf("current process CreatedAtUnknown = true, want false")
	}
	gotPath := filepath.Clean(found.Path)
	if !strings.EqualFold(gotPath, wantPath) {
		t.Fatalf("current process Path = %q, want %q (case-insensitive)", gotPath, wantPath)
	}
	if found.CreatedAt.IsZero() {
		t.Fatalf("current process CreatedAt is zero")
	}
	if found.CreatedAt.After(time.Now()) {
		t.Fatalf("current process CreatedAt %v is in the future", found.CreatedAt)
	}
}

func TestListCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	list, err := List(ctx)
	if err == nil {
		t.Fatalf("List with canceled ctx returned nil error, want context.Canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("List error = %v, want context.Canceled", err)
	}
	if list != nil {
		t.Fatalf("List with canceled ctx returned %d entries, want nil result on error", len(list))
	}
}

func TestListNoDuplicatePIDs(t *testing.T) {
	list, err := List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	seen := make(map[uint32]int, len(list))
	for _, p := range list {
		seen[p.PID]++
	}
	for pid, count := range seen {
		if count > 1 {
			t.Errorf("pid %d appears %d times in List result", pid, count)
		}
	}
}

func TestListHasReadablePaths(t *testing.T) {
	list, err := List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := 0
	for _, p := range list {
		if !p.PathUnknown && p.Path != "" {
			found++
		}
	}
	if found == 0 {
		t.Fatalf("List returned %d entries, none with a readable Path", len(list))
	}
}
