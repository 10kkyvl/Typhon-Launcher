package selfupdate

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sync"
	"testing"
)

func TestServiceStaysConsistentUnderConcurrentUserActions(t *testing.T) {
	stand := newReleaseStand(t, "1.2.3", bytes.Repeat([]byte("c"), 64*1024))
	dir := t.TempDir()
	s := standService(t, dir, stand, "1.0.0")
	s.status = Status{State: StateIdle, CurrentVersion: "1.0.0"}
	ctx := context.Background()

	tolerated := func(err error) bool {
		return err == nil ||
			errors.Is(err, ErrBusy) ||
			errors.Is(err, ErrNotReady) ||
			errors.Is(err, errNoUpdateChecked) ||
			errors.Is(err, context.Canceled)
	}
	ops := []struct {
		name string
		run  func() error
	}{
		{"check", func() error { _, err := s.CheckForUpdate(ctx); return err }},
		{"download", func() error { _, err := s.DownloadUpdate(ctx); return err }},
		{"cancel", func() error { return s.CancelDownload() }},
		{"dismiss", func() error { return s.DismissUpdate() }},
		{"status", func() error { s.GetStatus(); return nil }},
		{"notes", func() error { _, err := s.GetReleaseNotes(); return err }},
	}

	const workers, rounds = 6, 15
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				op := ops[(w+i)%len(ops)]
				if err := op.run(); !tolerated(err) {
					t.Errorf("%s: unexpected error %v", op.name, err)
				}
			}
		}()
	}
	wg.Wait()

	s.mu.Lock()
	busy, check := s.busy, s.check
	s.mu.Unlock()
	if busy || check != nil {
		t.Fatalf("busy=%v check=%v after every action returned: the service would refuse the next click", busy, check)
	}

	status := s.GetStatus()
	saved, err := mustStore(t, dir).Load()
	if err != nil {
		t.Fatalf("saved state unreadable after the run: %v", err)
	}
	switch status.State {
	case StateReady:
		if saved.Artifact == nil || saved.ReadyPath == "" {
			t.Fatalf("status is ready but the saved state is %+v: a restart would lose the download", saved)
		}
		if err := VerifyFile(ctx, saved.ReadyPath, *saved.Artifact); err != nil {
			t.Fatalf("status is ready but the installer on disk does not verify: %v", err)
		}
	case StateAvailable, StateIdle:
		if saved.ReadyPath != "" || saved.Artifact != nil {
			t.Fatalf("status is %s but the saved state still points at %+v", status.State, saved)
		}
		installerPath, err := ArtifactPath(dir, "1.2.3", stand.artifactName())
		if err != nil {
			t.Fatalf("ArtifactPath: %v", err)
		}
		if _, err := os.Stat(installerPath); err == nil {
			t.Fatalf("status is %s but a finished installer still sits at %s", status.State, installerPath)
		}
	default:
		t.Fatalf("status = %+v, want ready, available or idle once the dust settles", status)
	}
}
